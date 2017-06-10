package cache

import (
	"bytes"
	"math/rand"
	"path/filepath"
	"restic/crypto"
	"restic/test"
	"testing"
	"time"
)

func TestBlockReaderWriter(t *testing.T) {
	tempdir, cleanup := test.TempDir(t)
	defer cleanup()

	seed := time.Now().Unix()
	t.Logf("seed is %v", seed)
	rand.Seed(seed)

	key := crypto.NewRandomKey()

	bw, err := NewBlockWriter(filepath.Join(tempdir, "file"), key)
	if err != nil {
		t.Fatal(err)
	}

	var bufs [][]byte

	for i := 0; i < 20; i++ {
		seed := rand.Intn(2000)
		buf := test.Random(seed, 1<<21+seed)
		err := bw.Write(buf)
		if err != nil {
			t.Fatal(err)
		}

		bufs = append(bufs, buf)
	}

	if err := bw.Close(); err != nil {
		t.Fatal(err)
	}

	br, err := NewBlockReader(filepath.Join(tempdir, "file"), key)
	if err != nil {
		t.Fatal(err)
	}

	var buf []byte
	for i := 0; i < len(bufs); i++ {
		buf, err = br.Read(buf)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(buf, bufs[i]) {
			t.Errorf("wrong bytes returned, want %02x..., got %02x....", bufs[i][:20], buf[:20])
		}
	}

	if err = br.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBlockReaderWriterJSON(t *testing.T) {
	tempdir, cleanup := test.TempDir(t)
	defer cleanup()

	seed := time.Now().Unix()
	t.Logf("seed is %v", seed)
	rand.Seed(seed)

	key := crypto.NewRandomKey()

	bw, err := NewBlockWriter(filepath.Join(tempdir, "file"), key)
	if err != nil {
		t.Fatal(err)
	}

	type Item struct {
		Foo string `json:"x"`
		Bar uint   `json:"y"`
	}

	item1 := Item{Foo: "xxxx", Bar: 23}
	err = bw.WriteJSON(item1)
	if err != nil {
		t.Fatal(err)
	}

	if err := bw.Close(); err != nil {
		t.Fatal(err)
	}

	br, err := NewBlockReader(filepath.Join(tempdir, "file"), key)
	if err != nil {
		t.Fatal(err)
	}

	var item2 Item
	err = br.ReadJSON(&item2)
	if err != nil {
		t.Fatal(err)
	}

	if item1 != item2 {
		t.Fatalf("wrong data returned, want %#v, got %#v", item1, item2)
	}

	if err = br.Close(); err != nil {
		t.Fatal(err)
	}
}
