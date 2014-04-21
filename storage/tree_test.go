package storage_test

import (
	"bytes"
	"strings"
	"time"

	"github.com/fd0/khepri/storage"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func parseTime(str string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, str)
	if err != nil {
		panic(err)
	}

	return t
}

var _ = Describe("Tree", func() {
	var t *storage.Tree
	var raw string

	BeforeEach(func() {
		t = new(storage.Tree)
		t.Nodes = []storage.Node{
			storage.Node{
				Name:    "foobar",
				Mode:    0755,
				ModTime: parseTime("2014-04-20T22:16:54.161401+02:00"),
				User:    1000,
				Group:   1001,
				Content: []byte{0x41, 0x42, 0x43},
			},
			storage.Node{
				Name:    "baz",
				Mode:    0755,
				User:    1000,
				ModTime: parseTime("2014-04-20T22:16:54.161401+02:00"),
				Group:   1001,
				Content: []byte("\xde\xad\xbe\xef\xba\xdc\x0d\xe0"),
			},
		}

		raw = `{"nodes":[{"name":"foobar","mode":493,"mtime":"2014-04-20T22:16:54.161401+02:00","user":1000,"group":1001,"content":"414243"},{"name":"baz","mode":493,"mtime":"2014-04-20T22:16:54.161401+02:00","user":1000,"group":1001,"content":"deadbeefbadc0de0"}]}`
	})

	It("Should save", func() {
		var buf bytes.Buffer
		t.Save(&buf)
		Expect(strings.TrimRight(buf.String(), "\n")).To(Equal(raw))

		t2 := new(storage.Tree)
		err := t2.Restore(&buf)
		Expect(err).NotTo(HaveOccurred())

		// test tree for equality
		Expect(t2).To(Equal(t))

		// test nodes for equality
		for i, n := range t.Nodes {
			Expect(n.Content).To(Equal(t2.Nodes[i].Content))
		}
	})

	It("Should restore", func() {
		buf := bytes.NewBufferString(raw)
		t2 := new(storage.Tree)
		err := t2.Restore(buf)
		Expect(err).NotTo(HaveOccurred())

		// test if tree has correctly been restored
		Expect(t2).To(Equal(t))
	})
})
