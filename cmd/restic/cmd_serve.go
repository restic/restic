package main

import (
	"context"
	"fmt"
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

func respond(w http.ResponseWriter, title string, body string) {
	template := `
	<html>
	<head>
		<style>
			h1,h2,h3 {text-align:center; margin: 0.5em;}
			table {margin: 0 auto;border-collapse: collapse; }
			thead th {text-align: left; font-weight: bold;}
			tbody tr:hover {background: #eee;}
			table, td, tr, th { border: 1px solid black; padding: .1em .5em;}
			a.file:before {content: '\1F4C4'}
			a.dir:before {content: '\1F4C1'}
		</style>
	</head>
	<body><h1>%s</h1>%s</body>
	</html>`
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, template, title, body)
}

type tableItem struct {
	node *restic.Node
	Path string
}

type MyWebHandler struct {
	repo restic.Repository
	ctx  context.Context
}

func (h *MyWebHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(os.Stderr, "GET: %s\n", r.URL.Path)

	if r.URL.Path == "/" {
		body := "<table>"
		body += "<tr><th>ID</th><th>Time</th><th>Host</th><th>Tags</th><th>Paths</th></tr>"
		for snapshot := range FindFilteredSnapshots(h.ctx, h.repo.Backend(), h.repo, &restic.SnapshotFilter{}, nil) {
			body += fmt.Sprintf(
				"<tr><td><a href='/%s'>%s</a></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
				snapshot.ID().Str(),
				snapshot.ID().Str(),
				snapshot.Time,
				snapshot.Hostname,
				snapshot.Tags,
				snapshot.Paths,
			)
		}
		body += "</table>"
		respond(w, "Snapshots", body)
	} else {
		uri := strings.Split(r.URL.Path, "/")
		path := "/" + strings.Trim(strings.Join(uri[2:], "/"), "/")

		sn, err := restic.FindSnapshot(h.ctx, h.repo.Backend(), h.repo, uri[1])
		if err != nil {
			respond(w, "Error", "Snapshot not found")
			return
		}

		var items []tableItem

		walker.Walk(h.ctx, h.repo, *sn.Tree, nil, func(_ restic.ID, nodepath string, node *restic.Node, err error) (bool, error) {
			if err != nil {
				return false, err
			}
			if node == nil {
				return false, nil
			}
			if fs.HasPathPrefix(path, nodepath) {
				fullpath := "/" + sn.ID().Str() + "/" + strings.Trim(nodepath, "/")
				items = append(items, tableItem{node, fullpath})
			}
			if node.Type == "dir" && !fs.HasPathPrefix(nodepath, path) {
				return false, walker.ErrSkipNode
			}
			return false, nil
		})

		// Walker found a single item and it's a file, which implies path is referencing a file and we want to dump it
		if len(items) == 1 && items[0].node.Type == "file" {
			d := dump.New("zip", h.repo, w)
			d.WriteNode(h.ctx, items[0].node)
		} else {
			table_dirs := "<tr><td><a href='..'>..</a></td><td>Parent</td><td>-</td></tr>\n"
			table_files := ""
			for _, item := range items {
				row := fmt.Sprintf(
					"<tr><td><a class='%s' href='%s'>%s</a></td><td>%s</td><td>%d</td></tr>\n",
					item.node.Type,
					item.Path,
					item.node.Name,
					item.node.Type,
					item.node.Size,
				)
				if fs.HasPathPrefix(item.Path, r.URL.Path) {
					//
				} else if item.node.Type == "dir" {
					table_dirs += row
				} else {
					table_files += row
				}
			}
			title := sn.ID().Str() + ": " + path
			body := "<table><tr><th>Name</th><th>Type</th><th>Size</th></tr>" + table_dirs + table_files + "</table>"
			respond(w, title, body)
		}
	}
}

func runWebServer(ctx context.Context, opts WebOptions, gopts GlobalOptions, args []string) error {
	if len(args) > 0 {
		return errors.Fatal("this command does not accept additional arguments")
	}

	repo, err := OpenRepository(ctx, gopts)
	if err != nil {
		return err
	}

	err = repo.LoadIndex(ctx)
	if err != nil {
		return err
	}

	http.Handle("/", &MyWebHandler{repo, ctx})

	return http.ListenAndServe(opts.Listen, nil)
}
