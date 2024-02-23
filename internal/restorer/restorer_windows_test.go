//go:build windows
// +build windows

package restorer

import (
	"context"
	"math"
	"os"
	"path"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/restic/restic/internal/errors"
	"github.com/restic/restic/internal/repository"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/windows"
)

func getBlockCount(t *testing.T, filename string) int64 {
	libkernel32 := windows.NewLazySystemDLL("kernel32.dll")
	err := libkernel32.Load()
	rtest.OK(t, err)
	proc := libkernel32.NewProc("GetCompressedFileSizeW")
	err = proc.Find()
	rtest.OK(t, err)

	namePtr, err := syscall.UTF16PtrFromString(filename)
	rtest.OK(t, err)

	result, _, _ := proc.Call(uintptr(unsafe.Pointer(namePtr)), 0)

	const invalidFileSize = uintptr(4294967295)
	if result == invalidFileSize {
		return -1
	}

	return int64(math.Ceil(float64(result) / 512))
}

type AdsTestInfo struct {
	dirName         string
	fileOrder       []int
	fileStreamNames []string
	Overwrite       bool
}

type NamedNode struct {
	name string
	node Node
}

type OrderedSnapshot struct {
	nodes []NamedNode
}

type OrderedDir struct {
	Nodes   []NamedNode
	Mode    os.FileMode
	ModTime time.Time
}

func TestOrderedAdsFile(t *testing.T) {

	files := []string{"mainadsfile.text", "mainadsfile.text:datastream1:$DATA", "mainadsfile.text:datastream2:$DATA"}
	dataArray := []string{"Main file data.", "First data stream.", "Second data stream."}
	var tests = map[string]AdsTestInfo{
		"main-stream-first": {
			dirName: "dir", fileStreamNames: files,
			fileOrder: []int{0, 1, 2},
		},
		"second-stream-first": {
			dirName: "dir", fileStreamNames: files,
			fileOrder: []int{1, 0, 2},
		},
		"main-stream-first-already-exists": {
			dirName: "dir", fileStreamNames: files,
			fileOrder: []int{0, 1, 2},
			Overwrite: true,
		},
		"second-stream-first-already-exists": {
			dirName: "dir", fileStreamNames: files,
			fileOrder: []int{1, 0, 2},
			Overwrite: true,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tempdir := rtest.TempDir(t)

			nodes := getOrderedAdsNodes(test.dirName, test.fileOrder, test.fileStreamNames[:], dataArray)

			res := setup(t, nodes)

			if test.Overwrite {

				os.Mkdir(path.Join(tempdir, test.dirName), os.ModeDir)
				//Create existing files
				for _, f := range files {
					data := []byte("This is some dummy data.")

					filepath := path.Join(tempdir, test.dirName, f)
					// Write the data to the file
					err := os.WriteFile(path.Clean(filepath), data, 0644)
					rtest.OK(t, err)
				}
			}

			res.SelectFilter = adsConflictFilter

			err := res.RestoreTo(ctx, tempdir)
			rtest.OK(t, err)

			for _, fileIndex := range test.fileOrder {
				currentFile := test.fileStreamNames[fileIndex]

				fp := path.Join(tempdir, test.dirName, currentFile)

				fi, err1 := os.Stat(fp)
				rtest.Assert(t, !errors.Is(err1, os.ErrNotExist), "The file "+currentFile+" does not exist")

				size := fi.Size()
				rtest.Assert(t, size > 0, "The file "+currentFile+" exists but is empty")

				content, err := os.ReadFile(fp)
				rtest.OK(t, err)
				contentString := string(content)
				rtest.Assert(t, contentString == dataArray[fileIndex], "The file "+currentFile+" exists but the content is not overwritten")

			}
		})
	}
}

func getOrderedAdsNodes(dir string, order []int, allFileNames []string, dataArray []string) []NamedNode {

	getFileNodes := func() []NamedNode {
		nodes := []NamedNode{}

		for _, index := range order {
			file := allFileNames[index]
			nodes = append(nodes, NamedNode{
				name: file,
				node: File{
					ModTime: time.Now(),
					Data:    dataArray[index],
				},
			})
		}

		return nodes
	}

	return []NamedNode{
		{
			name: dir,
			node: OrderedDir{
				Mode:    normalizeFileMode(0750 | os.ModeDir),
				ModTime: time.Now(),
				Nodes:   getFileNodes(),
			},
		},
	}
}

func adsConflictFilter(item string, dstpath string, node *restic.Node) (selectedForRestore bool, childMayBeSelected bool) {
	switch filepath.ToSlash(item) {
	case "/dir":
		childMayBeSelected = true
	case "/dir/mainadsfile.text":
		selectedForRestore = true
		childMayBeSelected = false
	case "/dir/mainadsfile.text:datastream1:$DATA":
		selectedForRestore = true
		childMayBeSelected = false
	case "/dir/mainadsfile.text:datastream2:$DATA":
		selectedForRestore = true
		childMayBeSelected = false
	case "/dir/dir":
		selectedForRestore = true
		childMayBeSelected = true
	case "/dir/dir:dirstream1:$DATA":
		selectedForRestore = true
		childMayBeSelected = false
	case "/dir/dir:dirstream2:$DATA":
		selectedForRestore = true
		childMayBeSelected = false
	}
	return selectedForRestore, childMayBeSelected
}

func setup(t *testing.T, namedNodes []NamedNode) *Restorer {

	repo := repository.TestRepository(t)

	sn, _ := saveOrderedSnapshot(t, repo, OrderedSnapshot{
		nodes: namedNodes,
	})

	res := NewRestorer(repo, sn, false, nil)

	return res
}

func saveDirOrdered(t testing.TB, repo restic.Repository, namedNodes []NamedNode, inode uint64) restic.ID {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tree := &restic.Tree{}
	for _, namedNode := range namedNodes {
		name := namedNode.name
		n := namedNode.node
		inode++
		switch node := n.(type) {
		case File:
			fi := n.(File).Inode
			if fi == 0 {
				fi = inode
			}
			lc := n.(File).Links
			if lc == 0 {
				lc = 1
			}
			fc := []restic.ID{}
			if len(n.(File).Data) > 0 {
				fc = append(fc, saveFile(t, repo, node))
			}
			mode := node.Mode
			if mode == 0 {
				mode = 0644
			}
			err := tree.Insert(&restic.Node{
				Type:    "file",
				Mode:    mode,
				ModTime: node.ModTime,
				Name:    name,
				UID:     uint32(os.Getuid()),
				GID:     uint32(os.Getgid()),
				Content: fc,
				Size:    uint64(len(n.(File).Data)),
				Inode:   fi,
				Links:   lc,
			})
			rtest.OK(t, err)
		case Dir:
			id := saveDir(t, repo, node.Nodes, inode)

			mode := node.Mode
			if mode == 0 {
				mode = 0755
			}

			err := tree.Insert(&restic.Node{
				Type:    "dir",
				Mode:    mode,
				ModTime: node.ModTime,
				Name:    name,
				UID:     uint32(os.Getuid()),
				GID:     uint32(os.Getgid()),
				Subtree: &id,
			})
			rtest.OK(t, err)
		case OrderedDir:
			id := saveDirOrdered(t, repo, node.Nodes, inode)

			mode := node.Mode
			if mode == 0 {
				mode = 0755
			}

			err := tree.Insert(&restic.Node{
				Type:    "dir",
				Mode:    mode,
				ModTime: node.ModTime,
				Name:    name,
				UID:     uint32(os.Getuid()),
				GID:     uint32(os.Getgid()),
				Subtree: &id,
			})
			rtest.OK(t, err)
		default:
			t.Fatalf("unknown node type %T", node)
		}
	}

	id, err := restic.SaveTree(ctx, repo, tree)
	if err != nil {
		t.Fatal(err)
	}

	return id
}

func saveOrderedSnapshot(t testing.TB, repo restic.Repository, snapshot OrderedSnapshot) (*restic.Snapshot, restic.ID) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg, wgCtx := errgroup.WithContext(ctx)
	repo.StartPackUploader(wgCtx, wg)
	treeID := saveDirOrdered(t, repo, snapshot.nodes, 1000)
	err := repo.Flush(ctx)
	if err != nil {
		t.Fatal(err)
	}

	sn, err := restic.NewSnapshot([]string{"test"}, nil, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	sn.Tree = &treeID
	id, err := restic.SaveSnapshot(ctx, repo, sn)
	if err != nil {
		t.Fatal(err)
	}

	return sn, id
}
