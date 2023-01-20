package selfupdate

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	rtest "github.com/restic/restic/internal/test"
)

func TestExtractToFileZip(t *testing.T) {
	printf := func(string, ...interface{}) {}
	dir := t.TempDir()

	ext := "zip"
	data := []byte("Hello World!")

	// create dummy archive
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:               "example.exe",
		UncompressedSize64: uint64(len(data)),
	})
	rtest.OK(t, err)
	_, err = w.Write(data[:])
	rtest.OK(t, err)
	rtest.OK(t, zw.Close())

	// run twice to test creating a new file and overwriting
	for i := 0; i < 2; i++ {
		outfn := filepath.Join(dir, ext+"-out")
		rtest.OK(t, extractToFile(archive.Bytes(), "src."+ext, outfn, printf))

		outdata, err := os.ReadFile(outfn)
		rtest.OK(t, err)
		rtest.Assert(t, bytes.Equal(data[:], outdata), "%v contains wrong data", outfn)

		// overwrite to test the file is properly overwritten
		rtest.OK(t, os.WriteFile(outfn, []byte{1, 2, 3}, 0))
	}
}
