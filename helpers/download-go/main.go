package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/pflag"
)

func die(f string, args ...interface{}) {
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	fmt.Fprintf(os.Stderr, f, args...)
	os.Exit(1)
}

func msg(f string, args ...interface{}) {
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	fmt.Printf(f, args...)
}

func warn(f string, args ...interface{}) {
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	fmt.Fprintf(os.Stderr, f, args...)
}

func verbose(f string, args ...interface{}) {
	if !opts.Verbose {
		return
	}
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	fmt.Printf(f, args...)
}

func rm(file string) {
	err := os.Remove(file)

	if os.IsNotExist(err) {
		err = nil
	}

	if err != nil {
		die("error removing %v: %v", file, err)
	}
}

func mkdirall(dir string) {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		die("mkdirall(%v) returned error: %v", dir, err)
	}
}

func get(url string) string {
	res, err := http.Get(url)
	if err != nil {
		die("unable to request %v: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		die("unexpected status code for %v: %v (%v)", url, res.StatusCode, res.Status)
	}

	buf, err := ioutil.ReadAll(res.Body)
	if err != nil {
		die("error reading %v: %v", url, err)
	}

	err = res.Body.Close()
	if err != nil {
		die("error closing %v: %v", url, err)
	}

	return string(buf)
}

// downloadGoVersion downloads the version of the Go compiler specified by
// version (without the "go" prefix) and saves it to a temporary file which is
// then returned.
func downloadGoVersion(version, os, arch string) *os.File {
	ext := "tar.gz"
	if os == "windows" {
		ext = "zip"
	}

	f, err := ioutil.TempFile("", fmt.Sprintf("go-*.%v", ext))
	if err != nil {
		die("unable to create tempfile: %v", err)
	}

	url := fmt.Sprintf("https://golang.org/dl/go%v.%v-%v.%v", version, os, arch, ext)
	verbose("downloading %v", url)
	res, err := http.Get(url)
	if err != nil {
		die("error requesting %v: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		die("unexpected status code for %v: %v (%v)", url, res.StatusCode, res.Status)
	}

	ct := res.Header.Get("Content-Type")
	if ext == "tar.gz" && ct != "application/octet-stream" {
		die("unexpected content type %q, want %q", ct, "application/octet-stream")
	}
	if ext == "zip" && ct != "application/zip" {
		die("unexpected content type %q, want %q", ct, "application/zip")
	}

	_, err = io.Copy(f, res.Body)
	if err != nil {
		die("error downloading %v: %v", url, err)
	}

	err = res.Body.Close()
	if err != nil {
		die("error closing %v: %v", url, err)
	}

	return f
}

func createFileAt(filename string, mode os.FileMode, rd io.Reader) {
	mkdirall(filepath.Dir(filename))

	f, err := os.Create(filename)
	if err != nil {
		die("unable to create %v: %v", filename, err)
	}

	_, err = io.Copy(f, rd)
	if err != nil {
		die("write %v failed: %v", filename, err)
	}

	err = f.Close()
	if err != nil {
		die("closing %v returned error: %v", filename, err)
	}

	// make sure the file mode is not too wide
	mode &= 0755
	err = os.Chmod(filename, mode)
	if err != nil {
		die("chmod %v returned error: %v", filename, err)
	}
}

// extractTarGz extract the tar.gz file to the directory dest.
func extractTarGz(file, dest string) {
	f, err := os.Open(file)
	if err != nil {
		die("unable to open %v: %v", file, err)
	}

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		die("unable to open gzip reader for %v: %v", file, err)
	}

	tarReader := tar.NewReader(gzReader)

	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			die("unable to open tar reader: %v", err)
		}

		name := filepath.Join(string(filepath.Separator), hdr.Name)
		name = filepath.Join(dest, filepath.Clean(name))

		switch hdr.Typeflag {
		case tar.TypeReg:
			createFileAt(name, os.FileMode(hdr.Mode), tarReader)
		case tar.TypeDir:
			mkdirall(name)
		default:
			warn("%v unknown type %d, skipping", name, hdr.Typeflag)
		}
	}
}

// extractZip extracts the zip file into the directory dest.
func extractZip(file, dest string) {
	rd, err := zip.OpenReader(file)
	if err != nil {
		die("error opening zip %v: %v", file, err)
	}

	for _, item := range rd.File {
		name := filepath.Join(string(filepath.Separator), item.Name)
		name = filepath.Join(dest, filepath.Clean(name))

		f, err := item.Open()
		if err != nil {
			die("unable to open %v: %v", name, err)
		}

		switch {
		case item.Mode().IsDir():
			mkdirall(name)
		case item.Mode().IsRegular():
			createFileAt(name, item.Mode(), f)
		default:
			warn("%v unknown type %d, skipping", name, item.Mode())
		}

		err = f.Close()
		if err != nil {
			die("error closing %v: %v", name, err)
		}
	}

	err = rd.Close()
	if err != nil {
		die("error close %v: %v", file, err)
	}
}

var opts struct {
	Version    string
	SourceFile string
	TargetDir  string
	Verbose    bool
	OS, Arch   string
}

func init() {
	pflag.StringVar(&opts.Version, "version", "", "download `version` of Go (empty: use latest version)")
	pflag.StringVar(&opts.SourceFile, "source-file", "", "extract the `file` (default: download Go)")
	pflag.StringVar(&opts.TargetDir, "target-dir", "", "extract the compiler into `dir`")
	pflag.StringVar(&opts.OS, "os", runtime.GOOS, "download for `OS` (default: runtime.GOOS)")
	pflag.StringVar(&opts.Arch, "arch", runtime.GOARCH, "download for `arch` (default: runtime.GOARCH)")
	pflag.BoolVarP(&opts.Verbose, "verbose", "v", false, "be verbose")
	pflag.Parse()
}

func main() {
	if opts.TargetDir == "" {
		die("target directory not set, pass --target-dir")
	}

	if opts.Version == "" && opts.SourceFile == "" {
		verbose("requesting latest Go version")
		opts.Version = get("https://golang.org/VERSION?m=text")
	}

	if strings.HasPrefix(opts.Version, "go") {
		opts.Version = opts.Version[2:]
	}

	if opts.SourceFile == "" {
		msg("downloading Go %v", opts.Version)

		tempfile := downloadGoVersion(opts.Version, opts.OS, opts.Arch)
		msg("success, saved as %v", tempfile.Name())

		tempfile.Close()
		opts.SourceFile = tempfile.Name()

		defer func() {
			verbose("remove tempfile %v", tempfile.Name())
			rm(tempfile.Name())
		}()
	}

	msg("extract %v to %v", opts.SourceFile, opts.TargetDir)
	switch {
	case strings.HasSuffix(opts.SourceFile, ".zip"):
		extractZip(opts.SourceFile, opts.TargetDir)
	case strings.HasSuffix(opts.SourceFile, ".tar.gz"):
		extractTarGz(opts.SourceFile, opts.TargetDir)
	default:
		die("unknown extension for file %v", opts.SourceFile)
	}
}
