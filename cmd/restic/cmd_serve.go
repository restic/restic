package main

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"

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
	Time  string
	Host  string
	Tags  string
	Paths string
}

type IndexPage struct {
	Title string
	Rows []IndexRow
}

const IndexTpl = `<html>
<head>
<link rel="stylesheet" href="/style.css">
<title>Index :: restic</title>
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

type FilesPage struct {
	Title string
	Rows []FileRow
}

const FilesTpl = `<html>
<head>
<link rel="stylesheet" href="/style.css">
<title>Files :: restic</title>
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
</html>
`

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
	filesPage := template.Must(template.New("files").Parse(FilesTpl))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(os.Stderr, "GET: %s\n", r.URL.Path)
		uri := strings.Split(r.URL.Path, "/")

		// Static assets
		if r.URL.Path == "/style.css" {
			w.Header().Set("Cache-Control", "max-age=300")
			w.Write([]byte(StyleTxt))
			return
		}

		// Index page, list snapshots
		if r.URL.Path == "/" {
			var rows []IndexRow
			for sn := range FindFilteredSnapshots(ctx, repo.Backend(), repo, &restic.SnapshotFilter{}, nil) {
				rows = append(rows, IndexRow{"/" + sn.ID().Str(), sn.ID().Str(), sn.Time.String(), sn.Hostname, strings.Join(sn.Tags, ", "), strings.Join(sn.Paths, ", ")})
			}
			indexPage.Execute(w, IndexPage{"Snapshots", rows})
			return
		}
		
		// Snapshot page, list files
		if sn, _ := restic.FindSnapshot(ctx, repo.Backend(), repo, uri[1]); sn != nil {
			reqPath := "/" + strings.Trim(strings.Join(uri[2:], "/"), "/")
			items := make(map[string]*restic.Node)

			walker.Walk(ctx, repo, *sn.Tree, nil, func(_ restic.ID, nodepath string, node *restic.Node, err error) (bool, error) {
				if err != nil {
					return false, err
				}
				if node == nil {
					return false, nil
				}
				if fs.HasPathPrefix(reqPath, nodepath) {
					items[nodepath] = node
				}
				if node.Type == "dir" && !fs.HasPathPrefix(nodepath, reqPath) {
					return false, walker.ErrSkipNode
				}
				return false, nil
			})

			// Requested path is a file, dump it
			if node, ok := items[reqPath]; ok && node.Type == "file" {
				d := dump.New("zip", repo, w)
				d.WriteNode(ctx, node)
				return
			}
			
			// List snapshot content
			if len(items) > 0 {
				var dirs []FileRow
				var files []FileRow
				for path, node := range items {
					fullpath := "/" + sn.ID().Str() + "/" + path
					row := FileRow{fullpath, node.Name, node.Type, node.Size}
					if fs.HasPathPrefix(fullpath, r.URL.Path) {
						//
					} else if node.Type == "dir" {
						dirs = append(dirs, row)
					} else {
						files = append(files, row)
					}
				}
				filesPage.Execute(w, FilesPage{sn.ID().Str() + ": " + reqPath, append(dirs, files...)})
				return
			}
		}

		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	})

	fmt.Fprintf(os.Stdout, "Server started on http://%s\n", opts.Listen);
	return http.ListenAndServe(opts.Listen, nil)
}
