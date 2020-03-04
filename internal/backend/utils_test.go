package backend_test

import (
	"bytes"
	"context"
	"math/rand"
	"testing"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

const KiB = 1 << 10
const MiB = 1 << 20

func TestLoadAll(t *testing.T) {
	b := mem.New()
	var buf []byte

	for i := 0; i < 20; i++ {
		data := rtest.Random(23+i, rand.Intn(MiB)+500*KiB)

		id := restic.Hash(data)
		h := restic.Handle{Name: id.String(), Type: restic.DataFile}
		err := b.Save(context.TODO(), h, restic.NewByteReader(data))
		rtest.OK(t, err)

		buf, err := backend.LoadAll(context.TODO(), buf, b, restic.Handle{Type: restic.DataFile, Name: id.String()})
		rtest.OK(t, err)

		if len(buf) != len(data) {
			t.Errorf("length of returned buffer does not match, want %d, got %d", len(data), len(buf))
			continue
		}

		if !bytes.Equal(buf, data) {
			t.Errorf("wrong data returned")
			continue
		}
	}
}

func save(t testing.TB, be restic.Backend, buf []byte) restic.Handle {
	id := restic.Hash(buf)
	h := restic.Handle{Name: id.String(), Type: restic.DataFile}
	err := be.Save(context.TODO(), h, restic.NewByteReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestLoadAllAppend(t *testing.T) {
	b := mem.New()

	h1 := save(t, b, []byte("foobar test string"))
	randomData := rtest.Random(23, rand.Intn(MiB)+500*KiB)
	h2 := save(t, b, randomData)

	var tests = []struct {
		handle restic.Handle
		buf    []byte
		want   []byte
	}{
		{
			handle: h1,
			buf:    nil,
			want:   []byte("foobar test string"),
		},
		{
			handle: h1,
			buf:    []byte("xxx"),
			want:   []byte("foobar test string"),
		},
		{
			handle: h2,
			buf:    nil,
			want:   randomData,
		},
		{
			handle: h2,
			buf:    make([]byte, 0, 200),
			want:   randomData,
		},
		{
			handle: h2,
			buf:    []byte("foobarbaz"),
			want:   randomData,
		},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			buf, err := backend.LoadAll(context.TODO(), test.buf, b, test.handle)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(buf, test.want) {
				t.Errorf("wrong data returned, want %q, got %q", test.want, buf)
			}
		})
	}
}
