package main

import (
	"context"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/restic/restic/internal/dump"
	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

var cmdServe = &cobra.Command{
	Use:   "serve",
	Short: "runs a web server to browse a repository",
	Long: `
The serve command runs a web server to browse a repository.
`,
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runWebServer(cmd.Context(), serveOptions, globalOptions, args)
	},
}

type ServeOptions struct {
	Listen string
}

var serveOptions ServeOptions

func init() {
	cmdRoot.AddCommand(cmdServe)
	cmdFlags := cmdServe.Flags()
	cmdFlags.StringVarP(&serveOptions.Listen, "listen", "l", "localhost:3080", "set the listen host name and `address`")
}

type fileNode struct {
	Path string
	Node *restic.Node
}

func listNodes(ctx context.Context, repo restic.Repository, tree restic.ID, path string) ([]fileNode, error) {
	var files []fileNode
	err := walker.Walk(ctx, repo, tree, walker.WalkVisitor{
		ProcessNode: func(parentTreeID restic.ID, nodepath string, node *restic.Node, err error) error {
			if err != nil || node == nil {
				return err
			}
			if fs.HasPathPrefix(path, nodepath) {
				files = append(files, fileNode{nodepath, node})
			}
			if node.Type == "dir" && !fs.HasPathPrefix(nodepath, path) {
				return walker.ErrSkipNode
			}
			return nil
		},
	})
	return files, err
}

func runWebServer(ctx context.Context, opts ServeOptions, gopts GlobalOptions, args []string) error {
	if len(args) > 0 {
		return errors.Fatal("this command does not accept additional arguments")
	}

	ctx, repo, unlock, err := openWithExclusiveLock(ctx, gopts, gopts.NoLock)
	if err != nil {
		return err
	}
	defer unlock()

	snapshotLister, err := restic.MemorizeList(ctx, repo, restic.SnapshotFile)
	if err != nil {
		return err
	}
	bar := newIndexProgress(gopts.Quiet, gopts.JSON)
	err = repo.LoadIndex(ctx, bar)
	if err != nil {
		return err
	}

	funcMap := template.FuncMap{
		"FormatTime": func(time time.Time) string { return time.Format(TimeFormat) },
	}
	indexPage := template.Must(template.New("index").Funcs(funcMap).Parse(indexPageTpl))
	treePage := template.Must(template.New("tree").Funcs(funcMap).Parse(treePageTpl))

	http.HandleFunc("/tree/", func(w http.ResponseWriter, r *http.Request) {
		snapshotID, curPath, _ := strings.Cut(r.URL.Path[6:], "/")
		curPath = "/" + strings.Trim(curPath, "/")
		_ = r.ParseForm()

		sn, _, err := restic.FindSnapshot(ctx, snapshotLister, repo, snapshotID)
		if err != nil {
			http.Error(w, "Snapshot not found: "+err.Error(), http.StatusNotFound)
			return
		}

		files, err := listNodes(ctx, repo, *sn.Tree, curPath)
		if err != nil || len(files) == 0 {
			http.Error(w, "Path not found in snapshot", http.StatusNotFound)
			return
		}

		if r.Form.Get("action") == "dump" {
			var tree restic.Tree
			for _, file := range files {
				for _, name := range r.Form["name"] {
					if name == file.Node.Name {
						tree.Nodes = append(tree.Nodes, file.Node)
					}
				}
			}
			if len(tree.Nodes) > 0 {
				filename := strings.ReplaceAll(strings.Trim(snapshotID+curPath, "/"), "/", "_") + ".tar.gz"
				w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
				// For now it's hardcoded to tar because it's the only format that supports all node types correctly
				if err := dump.New("tar", repo, w).DumpTree(ctx, &tree, "/"); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}
		}

		if len(files) == 1 && files[0].Node.Type == "file" {
			if err := dump.New("zip", repo, w).WriteNode(ctx, files[0].Node); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		var rows []treePageRow
		for _, item := range files {
			if item.Path != curPath {
				rows = append(rows, treePageRow{"/tree/" + snapshotID + item.Path, item.Node.Name, item.Node.Type, item.Node.Size, item.Node.ModTime})
			}
		}
		sort.SliceStable(rows, func(i, j int) bool {
			return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
		})
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].Type == "dir" && rows[j].Type != "dir"
		})
		parent := "/tree/" + snapshotID + curPath + "/.."
		if curPath == "/" {
			parent = "/"
		}
		if err := treePage.Execute(w, treePageData{snapshotID + ": " + curPath, parent, rows}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		var rows []indexPageRow
		for sn := range FindFilteredSnapshots(ctx, snapshotLister, repo, &restic.SnapshotFilter{}, nil) {
			rows = append(rows, indexPageRow{"/tree/" + sn.ID().Str() + "/", sn.ID().Str(), sn.Time, sn.Hostname, sn.Tags, sn.Paths})
		}
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].Time.After(rows[j].Time)
		})
		if err := indexPage.Execute(w, indexPageData{"Snapshots", rows}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=300")
		_, _ = w.Write([]byte(stylesheetTxt))
	})

	Printf("Now serving the repository at http://%s\n", opts.Listen)
	Printf("When finished, quit with Ctrl-c here.\n")

	return http.ListenAndServe(opts.Listen, nil)
}

type indexPageRow struct {
	Link  string
	ID    string
	Time  time.Time
	Host  string
	Tags  []string
	Paths []string
}

type indexPageData struct {
	Title string
	Rows  []indexPageRow
}

type treePageRow struct {
	Link string
	Name string
	Type string
	Size uint64
	Time time.Time
}

type treePageData struct {
	Title  string
	Parent string
	Rows   []treePageRow
}

const indexPageTpl = `<html>
<head>
<link rel="stylesheet" href="/style.css">
<title>{{.Title}} :: restic</title>
</head>
<body>
<h1>{{.Title}}</h1>
<table>
<thead><tr><th>ID</th><th>Time</th><th>Host</th><th>Tags</th><th>Paths</th></tr></thead>
<tbody>
{{range .Rows}}
<tr><td><a href="{{.Link}}">{{.ID}}</a></td><td>{{.Time | FormatTime}}</td><td>{{.Host}}</td><td>{{.Tags}}</td><td>{{.Paths}}</td></tr>
{{end}}
</tbody>
</table>
</body>
</html>`

const treePageTpl = `<html>
<head>
<link rel="stylesheet" href="/style.css">
<title>{{.Title}} :: restic</title>
</head>
<body>
<h1>{{.Title}}</h1>
<form method="post">
<table>
<thead><tr><th><input type="checkbox" onclick="document.querySelectorAll('.content input[type=checkbox]').forEach(cb => cb.checked = this.checked)"></th><th>Name</th><th>Type</th><th>Size</th><th>Date modified</th></tr></thead>
<tbody class="content">
{{if .Parent}}<tr><td></td><td><a href="{{.Parent}}">..</a></td><td>parent</td><td></td><td></tr>{{end}}
{{range .Rows}}
<tr><td><input type="checkbox" name="name" value="{{.Name}}"></td><td><a class="{{.Type}}" href="{{.Link}}">{{.Name}}</a></td><td>{{.Type}}</td><td>{{.Size}}</td><td>{{.Time | FormatTime}}</td></td></tr>
{{end}}
</tbody>
<tbody class="actions">
<tr><td colspan="100"><button name="action" value="dump" type="submit">Download selection</button></td></tr>
</tbody>
</table>
</form>
</body>
</html>`

const stylesheetTxt = `
h1,h2,h3 {text-align:center; margin: 0.5em;}
table {margin: 0 auto;border-collapse: collapse; }
thead th {text-align: left; font-weight: bold;}
tbody.content tr:hover {background: #eee;}
tbody.content a.file:before {content: '\1F4C4'}
tbody.content a.dir:before {content: '\1F4C1'}
tbody.actions td {padding:.5em;}
table, td, tr, th { border: 1px solid black; padding: .1em .5em;}
`
