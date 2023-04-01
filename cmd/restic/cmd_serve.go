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
		return runWebServer(cmd.Context(), cmdOptions, globalOptions, args)
	},
}

type WebOptions struct {
	Listen string
}

var cmdOptions WebOptions

func init() {
	cmdRoot.AddCommand(cmdServe)
	cmdFlags := cmdServe.Flags()
	cmdFlags.StringVarP(&cmdOptions.Listen, "listen", "l", "localhost:3080", "set the listen host name and `address`")
}

const StyleTxt = `
h1,h2,h3 {text-align:center; margin: 0.5em;}
table {margin: 0 auto;border-collapse: collapse; }
thead th {text-align: left; font-weight: bold;}
tbody tr:hover {background: #eee;}
table, td, tr, th { border: 1px solid black; padding: .1em .5em;}
a.file:before {content: '\1F4C4'}
a.dir:before {content: '\1F4C1'}
`

type IndexRow struct {
	Link  string
	ID    string
	Time  time.Time
	Host  string
	Tags  []string
	Paths []string
}

type IndexPage struct {
	Title string
	Rows  []IndexRow
}

const IndexTpl = `<html>
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
<tr><td><a href="{{.Link}}">{{.ID}}</a></td><td>{{.Time}}</td><td>{{.Host}}</td><td>{{.Tags}}</td><td>{{.Paths}}</td></tr>
{{end}}
</tbody>
</table>
</body>
</html>`

type FileRow struct {
	Link string
	Name string
	Type string
	Size uint64
}

type TreePage struct {
	Title string
	Rows  []FileRow
}

const TreeTpl = `<html>
<head>
<link rel="stylesheet" href="/style.css">
<title>{{.Title}} :: restic</title>
</head>
<body>
<h1>{{.Title}}</h1>
<table>
<thead><tr><th>Name</th><th>Type</th><th>Size</th></tr></thead>
<tbody>
<tr><td><a href='..'>..</a></td><td>Parent</td><td>-</td></tr>
{{range .Rows}}
<tr><td><a class="{{.Type}}" href="{{.Link}}">{{.Name}}</a></td><td>{{.Type}}</td><td>{{.Size}}</td></tr>
{{end}}
</tbody>
</table>
</body>
</html>`

type NodePath struct {
	Path string
	Node *restic.Node
}

func listNodes(ctx context.Context, repo restic.Repository, sn *restic.Snapshot, path string) ([]NodePath, error) {
	var items []NodePath
	err := walker.Walk(ctx, repo, *sn.Tree, nil, func(_ restic.ID, nodepath string, node *restic.Node, err error) (bool, error) {
		if err != nil {
			return false, err
		}
		if node == nil {
			return false, nil
		}
		if fs.HasPathPrefix(path, nodepath) {
			items = append(items, NodePath{nodepath, node})
		}
		if node.Type == "dir" && !fs.HasPathPrefix(nodepath, path) {
			return false, walker.ErrSkipNode
		}
		return false, nil
	})
	return items, err
}

func runWebServer(ctx context.Context, opts WebOptions, gopts GlobalOptions, args []string) error {
	if len(args) > 0 {
		return errors.Fatal("this command does not accept additional arguments")
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	if !gopts.NoLock {
		var lock *restic.Lock
		lock, ctx, err = lockRepo(ctx, repo)
		defer unlockRepo(lock)
		if err != nil {
			return err
		}
	}

	err = repo.LoadIndex(ctx)
	if err != nil {
		return err
	}

	indexPage := template.Must(template.New("index").Parse(IndexTpl))
	treePage := template.Must(template.New("tree").Parse(TreeTpl))

	http.HandleFunc("/tree/", func(w http.ResponseWriter, r *http.Request) {
		uriParts := strings.Split(r.URL.Path[1:], "/")
		if len(uriParts) < 3 {
			http.Redirect(w, r, "/", http.StatusMovedPermanently)
			return
		}

		snapshotID := uriParts[1]
		curPath := "/" + strings.Join(uriParts[2:], "/")

		sn, err := restic.FindSnapshot(ctx, repo.Backend(), repo, snapshotID)
		if err != nil {
			http.Error(w, "Snapshot not found: "+err.Error(), http.StatusNotFound)
			return
		}

		items, err := listNodes(ctx, repo, sn, curPath)
		if err != nil || len(items) == 0 {
			http.Error(w, "Path not found in snapshot", http.StatusNotFound)
			return
		}

		if len(items) == 1 && items[0].Node.Type == "file" {
			// Requested path is a file, dump it
			if err := dump.New("zip", repo, w).WriteNode(ctx, items[0].Node); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		} else {
			// Requested path is a folder, list it
			var files []FileRow
			for _, item := range items {
				if !fs.HasPathPrefix(item.Path, curPath) {
					files = append(files, FileRow{"/tree/" + snapshotID + item.Path, item.Node.Name, item.Node.Type, item.Node.Size})
				}
			}
			sort.SliceStable(files, func(i, j int) bool {
				return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
			})
			sort.SliceStable(files, func(i, j int) bool {
				return files[i].Type == "dir" && files[j].Type != "dir"
			})
			if err := treePage.Execute(w, TreePage{snapshotID + ": " + curPath, files}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		var rows []IndexRow
		for sn := range FindFilteredSnapshots(ctx, repo.Backend(), repo, &restic.SnapshotFilter{}, nil) {
			rows = append(rows, IndexRow{"/tree/" + sn.ID().Str() + "/", sn.ID().Str(), sn.Time, sn.Hostname, sn.Tags, sn.Paths})
		}
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].Time.After(rows[j].Time)
		})
		if err := indexPage.Execute(w, IndexPage{"Snapshots", rows}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=300")
		_, _ = w.Write([]byte(StyleTxt))
	})

	Printf("Now serving the repository at http://%s\n", opts.Listen)
	Printf("When finished, quit with Ctrl-c here.\n")

	return http.ListenAndServe(opts.Listen, nil)
}
