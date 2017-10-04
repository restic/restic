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

	for i := 0; i < 20; i++ {
		data := rtest.Random(23+i, rand.Intn(MiB)+500*KiB)

		id := restic.Hash(data)
		err := b.Save(context.TODO(), restic.Handle{Name: id.String(), Type: restic.DataFile}, bytes.NewReader(data))
		rtest.OK(t, err)

		buf, err := backend.LoadAll(context.TODO(), b, restic.Handle{Type: restic.DataFile, Name: id.String()})
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

func TestLoadSmallBuffer(t *testing.T) {
	b := mem.New()

	for i := 0; i < 20; i++ {
		data := rtest.Random(23+i, rand.Intn(MiB)+500*KiB)

		id := restic.Hash(data)
		err := b.Save(context.TODO(), restic.Handle{Name: id.String(), Type: restic.DataFile}, bytes.NewReader(data))
		rtest.OK(t, err)

		buf, err := backend.LoadAll(context.TODO(), b, restic.Handle{Type: restic.DataFile, Name: id.String()})
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

func TestLoadLargeBuffer(t *testing.T) {
	b := mem.New()

	for i := 0; i < 20; i++ {
		data := rtest.Random(23+i, rand.Intn(MiB)+500*KiB)

		id := restic.Hash(data)
		err := b.Save(context.TODO(), restic.Handle{Name: id.String(), Type: restic.DataFile}, bytes.NewReader(data))
		rtest.OK(t, err)

		buf, err := backend.LoadAll(context.TODO(), b, restic.Handle{Type: restic.DataFile, Name: id.String()})
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
