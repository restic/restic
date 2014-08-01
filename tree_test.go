package khepri_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/fd0/khepri"
)

func parseTime(str string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, str)
	if err != nil {
		panic(err)
	}

	return t
}

func TestTree(t *testing.T) {
	var tree = &khepri.Tree{
		Nodes: []khepri.Node{
			khepri.Node{
				Name:       "foobar",
				Mode:       0755,
				ModTime:    parseTime("2014-04-20T22:16:54.161401+02:00"),
				AccessTime: parseTime("2014-04-21T22:16:54.161401+02:00"),
				User:       1000,
				Group:      1001,
				Content:    []byte{0x41, 0x42, 0x43},
			},
			khepri.Node{
				Name:       "baz",
				Mode:       0755,
				User:       1000,
				ModTime:    parseTime("2014-04-20T22:16:54.161401+02:00"),
				AccessTime: parseTime("2014-04-21T22:16:54.161401+02:00"),
				Group:      1001,
				Content:    []byte("\xde\xad\xbe\xef\xba\xdc\x0d\xe0"),
			},
		},
	}

	const raw = `{"nodes":[{"name":"foobar","mode":493,"mtime":"2014-04-20T22:16:54.161401+02:00","atime":"2014-04-21T22:16:54.161401+02:00","user":1000,"group":1001,"content":"414243"},{"name":"baz","mode":493,"mtime":"2014-04-20T22:16:54.161401+02:00","atime":"2014-04-21T22:16:54.161401+02:00","user":1000,"group":1001,"content":"deadbeefbadc0de0"}]}`

	// test save
	buf := &bytes.Buffer{}

	tree.Save(buf)
	equals(t, raw, strings.TrimRight(buf.String(), "\n"))

	tree2 := new(khepri.Tree)
	err := tree2.Restore(buf)
	ok(t, err)
	equals(t, tree, tree2)

	// test nodes for equality
	for i, n := range tree.Nodes {
		equals(t, n.Content, tree2.Nodes[i].Content)
	}

	// test restore
	buf = bytes.NewBufferString(raw)

	tree2 = new(khepri.Tree)
	err = tree2.Restore(buf)
	ok(t, err)

	// test if tree has correctly been restored
	equals(t, tree, tree2)
}
