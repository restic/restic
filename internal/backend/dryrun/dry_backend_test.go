package dryrun_test

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/restic/restic/internal/restic"

	"github.com/restic/restic/internal/backend/dryrun"
	"github.com/restic/restic/internal/backend/mem"
)

// make sure that Backend implements backend.Backend
var _ restic.Backend = &dryrun.Backend{}

func newBackends() (*dryrun.Backend, restic.Backend) {
	m := mem.New()
	return dryrun.New(m), m
}

func TestDry(t *testing.T) {
	ctx := context.TODO()

	d, m := newBackends()
	// Since the dry backend is a mostly write-only overlay, the standard backend test suite
	// won't pass. Instead, perform a series of operations over the backend, testing the state
	// at each step.
	steps := []struct {
		be      restic.Backend
		op      string
		fname   string
		content string
		wantErr string
	}{
		{d, "loc", "", "DRY:RAM", ""},
		{d, "delete", "", "", ""},
		{d, "stat", "a", "", "not found"},
		{d, "list", "", "", ""},
		{d, "save", "", "", "invalid"},
		{m, "save", "a", "baz", ""},  // save a directly to the mem backend
		{d, "save", "b", "foob", ""}, // b is not saved
		{d, "save", "b", "xxx", ""},  // no error as b is not saved
		{d, "stat", "", "", "invalid"},
		{d, "stat", "a", "a 3", ""},
		{d, "load", "a", "baz", ""},
		{d, "load", "b", "", "not found"},
		{d, "list", "", "a", ""},
		{d, "remove", "c", "", ""},
		{d, "stat", "b", "", "not found"},
		{d, "list", "", "a", ""},
		{d, "remove", "a", "", ""}, // a is in fact not removed
		{d, "list", "", "a", ""},
		{m, "remove", "a", "", ""}, // remove a from the mem backend
		{d, "list", "", "", ""},
		{d, "close", "", "", ""},
		{d, "close", "", "", ""},
	}

	for i, step := range steps {
		var err error

		handle := restic.Handle{Type: restic.PackFile, Name: step.fname}
		switch step.op {
		case "save":
			err = step.be.Save(ctx, handle, restic.NewByteReader([]byte(step.content), step.be.Hasher()))
		case "list":
			fileList := []string{}
			err = step.be.List(ctx, restic.PackFile, func(fi restic.FileInfo) error {
				fileList = append(fileList, fi.Name)
				return nil
			})
			sort.Strings(fileList)
			files := strings.Join(fileList, " ")
			if files != step.content {
				t.Errorf("%d. List = %q, want %q", i, files, step.content)
			}
		case "loc":
			loc := step.be.Location()
			if loc != step.content {
				t.Errorf("%d. Location = %q, want %q", i, loc, step.content)
			}
		case "delete":
			err = step.be.Delete(ctx)
		case "remove":
			err = step.be.Remove(ctx, handle)
		case "stat":
			var fi restic.FileInfo
			fi, err = step.be.Stat(ctx, handle)
			if err == nil {
				fis := fmt.Sprintf("%s %d", fi.Name, fi.Size)
				if fis != step.content {
					t.Errorf("%d. Stat = %q, want %q", i, fis, step.content)
				}
			}
		case "load":
			data := ""
			err = step.be.Load(ctx, handle, 100, 0, func(rd io.Reader) error {
				buf, err := io.ReadAll(rd)
				data = string(buf)
				return err
			})
			if data != step.content {
				t.Errorf("%d. Load = %q, want %q", i, data, step.content)
			}
		case "close":
			err = step.be.Close()
		default:
			t.Fatalf("%d. unknown step operation %q", i, step.op)
		}
		if step.wantErr != "" {
			if err == nil {
				t.Errorf("%d. %s error = nil, want %q", i, step.op, step.wantErr)
			} else if !strings.Contains(err.Error(), step.wantErr) {
				t.Errorf("%d. %s error = %q, doesn't contain %q", i, step.op, err, step.wantErr)
			} else if step.wantErr == "not found" && !step.be.IsNotExist(err) {
				t.Errorf("%d. IsNotExist(%s error) = false, want true", i, step.op)
			}

		} else if err != nil {
			t.Errorf("%d. %s error = %q, want nil", i, step.op, err)
		}
	}
}
