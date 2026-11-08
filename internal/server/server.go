// Package server contains an HTTP server which can serve content from a repo.
package server

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/restic/restic/internal/dump"
	rfs "github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/restic"
	"github.com/restic/restic/internal/walker"
)

//go:embed assets/*.html assets/*.css
var assets embed.FS

// New returns a new HTTP server.
func New(repo restic.Repository, snapshotLister restic.Lister, timeFormat string) (http.Handler, error) {
	assetsFS, err := fs.Sub(assets, "assets")
	if err != nil {
		return nil, fmt.Errorf("derive subdir fs for assets: %w", err)
	}

	funcs := template.FuncMap{
		"FormatTime": func(time time.Time) string { return time.Format(timeFormat) },
	}

	templates := template.Must(template.New("").Funcs(funcs).ParseFS(assetsFS, "*.html"))

	mux := http.NewServeMux()

	indexPage := templates.Lookup("index.html")
	if indexPage == nil {
		panic("index.html not found")
	}

	treePage := templates.Lookup("tree.html")
	if treePage == nil {
		panic("tree.html not found")
	}

	mux.HandleFunc("/tree/", func(rw http.ResponseWriter, req *http.Request) {
		snapshotID, curPath, _ := strings.Cut(req.URL.Path[6:], "/")
		curPath = "/" + strings.Trim(curPath, "/")
		_ = req.ParseForm()

		sn, _, err := restic.FindSnapshot(req.Context(), snapshotLister, repo, snapshotID)
		if err != nil {
			http.Error(rw, "Snapshot not found: "+err.Error(), http.StatusNotFound)
			return
		}

		files, err := listNodes(req.Context(), repo, *sn.Tree, curPath)
		if err != nil || len(files) == 0 {
			http.Error(rw, "Path not found in snapshot", http.StatusNotFound)
			return
		}

		if req.Form.Get("action") == "dump" {
			var tree restic.Tree
			for _, file := range files {
				for _, name := range req.Form["name"] {
					if name == file.Node.Name {
						tree.Nodes = append(tree.Nodes, file.Node)
					}
				}
			}
			if len(tree.Nodes) > 0 {
				filename := strings.ReplaceAll(strings.Trim(snapshotID+curPath, "/"), "/", "_") + ".tar.gz"
				rw.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
				// For now it's hardcoded to tar because it's the only format that supports all node types correctly
				if err := dump.New("tar", repo, rw).DumpTree(req.Context(), &tree, "/"); err != nil {
					http.Error(rw, err.Error(), http.StatusInternalServerError)
				}
				return
			}
		}

		if len(files) == 1 && files[0].Node.Type == "file" {
			if err := dump.New("zip", repo, rw).WriteNode(req.Context(), files[0].Node); err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		var rows []treePageRow
		for _, item := range files {
			if item.Path != curPath {
				rows = append(rows, treePageRow{
					Link: "/tree/" + snapshotID + item.Path,
					Name: item.Node.Name,
					Type: item.Node.Type,
					Size: item.Node.Size,
					Time: item.Node.ModTime,
				})
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
		if err := treePage.Execute(rw, treePageData{snapshotID + ": " + curPath, parent, rows}); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(rw, req)
			return
		}

		var rows []indexPageRow
		for sn := range findFilteredSnapshots(req.Context(), snapshotLister, repo, &restic.SnapshotFilter{}, nil) {
			rows = append(rows, indexPageRow{
				Link:  "/tree/" + sn.ID().Str() + "/",
				ID:    sn.ID().Str(),
				Time:  sn.Time,
				Host:  sn.Hostname,
				Tags:  sn.Tags,
				Paths: sn.Paths,
			})
		}

		sort.Slice(rows, func(i, j int) bool {
			return rows[i].Time.After(rows[j].Time)
		})

		if err := indexPage.Execute(rw, indexPageData{"Snapshots", rows}); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("/style.css", func(rw http.ResponseWriter, req *http.Request) {
		buf, err := fs.ReadFile(assetsFS, "style.css")
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)

			fmt.Fprintf(rw, "error reading embedded style.css: %v\n", err)

			return
		}

		rw.Header().Set("Cache-Control", "max-age=300")
		rw.Header().Set("Content-Type", "text/css")

		_, _ = rw.Write(buf)
	})

	return mux, nil
}

type fileNode struct {
	Path string
	Node *restic.Node
}

func listNodes(ctx context.Context, repo restic.Repository, tree restic.ID, path string) ([]fileNode, error) {
	var files []fileNode
	err := walker.Walk(ctx, repo, tree, walker.WalkVisitor{
		ProcessNode: func(_ restic.ID, nodepath string, node *restic.Node, err error) error {
			if err != nil || node == nil {
				return err
			}
			if rfs.HasPathPrefix(path, nodepath) {
				files = append(files, fileNode{nodepath, node})
			}
			if node.Type == "dir" && !rfs.HasPathPrefix(nodepath, path) {
				return walker.ErrSkipNode
			}
			return nil
		},
	})
	return files, err
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

// findFilteredSnapshots yields Snapshots, either given explicitly by `snapshotIDs` or filtered from the list of all snapshots.
func findFilteredSnapshots(ctx context.Context, be restic.Lister, loader restic.LoaderUnpacked, f *restic.SnapshotFilter, snapshotIDs []string) <-chan *restic.Snapshot {
	out := make(chan *restic.Snapshot)
	go func() {
		defer close(out)
		be, err := restic.MemorizeList(ctx, be, restic.SnapshotFile)
		if err != nil {
			// Warnf("could not load snapshots: %v\n", err)
			return
		}

		err = f.FindAll(ctx, be, loader, snapshotIDs, func(id string, sn *restic.Snapshot, err error) error {
			if err != nil {
				// Warnf("Ignoring %q: %v\n", id, err)
			} else {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case out <- sn:
				}
			}
			return nil
		})
		if err != nil {
			// Warnf("could not load snapshots: %v\n", err)
		}
	}()
	return out
}
