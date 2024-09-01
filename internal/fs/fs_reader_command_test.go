package fs_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/restic/restic/internal/fs"
	"github.com/restic/restic/internal/test"
)

func TestCommandReaderSuccess(t *testing.T) {
	reader, err := fs.NewCommandReader(context.TODO(), []string{"true"}, io.Discard)
	test.OK(t, err)

	_, err = io.Copy(io.Discard, reader)
	test.OK(t, err)

	test.OK(t, reader.Close())
}

func TestCommandReaderFail(t *testing.T) {
	reader, err := fs.NewCommandReader(context.TODO(), []string{"false"}, io.Discard)
	test.OK(t, err)

	_, err = io.Copy(io.Discard, reader)
	test.Assert(t, err != nil, "missing error")
}

func TestCommandReaderInvalid(t *testing.T) {
	_, err := fs.NewCommandReader(context.TODO(), []string{"w54fy098hj7fy5twijouytfrj098y645wr"}, io.Discard)
	test.Assert(t, err != nil, "missing error")
}

func TestCommandReaderEmptyArgs(t *testing.T) {
	_, err := fs.NewCommandReader(context.TODO(), []string{}, io.Discard)
	test.Assert(t, err != nil, "missing error")
}

func TestCommandReaderOutput(t *testing.T) {
	reader, err := fs.NewCommandReader(context.TODO(), []string{"echo", "hello world"}, io.Discard)
	test.OK(t, err)

	var buf bytes.Buffer

	_, err = io.Copy(&buf, reader)
	test.OK(t, err)
	test.OK(t, reader.Close())

	test.Equals(t, "hello world", strings.TrimSpace(buf.String()))
}
