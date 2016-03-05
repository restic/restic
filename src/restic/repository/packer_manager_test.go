package repository

import (
	"io"
	"math/rand"
	"restic/backend"
	"restic/backend/mem"
	"restic/crypto"
	"restic/pack"
	"testing"
)

func randomID(rd io.Reader) backend.ID {
	id := backend.ID{}
	_, err := io.ReadFull(rd, id[:])
	if err != nil {
		panic(err)
	}
	return id
}

func fillPacks(t testing.TB, rnd *rand.Rand, be Saver, pm *packerManager) (bytes int) {
	for i := 0; i < 100; i++ {
		l := rnd.Intn(1 << 20)
		seed := rnd.Int63()

		packer, err := pm.findPacker(uint(l))
		if err != nil {
			t.Fatal(err)
		}

		rd := rand.New(rand.NewSource(seed))
		id := randomID(rd)
		n, err := packer.Add(pack.Data, id, io.LimitReader(rd, int64(l)))

		if n != int64(l) {
			t.Errorf("Add() returned invalid number of bytes: want %v, got %v", n, l)
		}
		bytes += l

		if packer.Size() < minPackSize && pm.countPacker() < maxPackers {
			pm.insertPacker(packer)
			continue
		}

		data, err := packer.Finalize()
		if err != nil {
			t.Fatal(err)
		}

		h := backend.Handle{Type: backend.Data, Name: randomID(rd).String()}

		err = be.Save(h, data)
		if err != nil {
			t.Fatal(err)
		}
	}

	return bytes
}

func flushRemainingPacks(t testing.TB, rnd *rand.Rand, be Saver, pm *packerManager) (bytes int) {
	if pm.countPacker() > 0 {
		for _, packer := range pm.packs {
			data, err := packer.Finalize()
			if err != nil {
				t.Fatal(err)
			}
			bytes += len(data)

			h := backend.Handle{Type: backend.Data, Name: randomID(rnd).String()}

			err = be.Save(h, data)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	return bytes
}

type fakeBackend struct{}

func (f *fakeBackend) Save(h backend.Handle, data []byte) error {
	return nil
}

func TestPackerManager(t *testing.T) {
	rnd := rand.New(rand.NewSource(23))

	be := mem.New()
	pm := &packerManager{
		be:  be,
		key: crypto.NewRandomKey(),
	}

	bytes := fillPacks(t, rnd, be, pm)
	bytes += flushRemainingPacks(t, rnd, be, pm)

	t.Logf("saved %d bytes", bytes)
}

func BenchmarkPackerManager(t *testing.B) {
	rnd := rand.New(rand.NewSource(23))

	be := &fakeBackend{}
	pm := &packerManager{
		be:  be,
		key: crypto.NewRandomKey(),
	}

	t.ResetTimer()

	bytes := 0
	for i := 0; i < t.N; i++ {
		bytes += fillPacks(t, rnd, be, pm)
	}

	bytes += flushRemainingPacks(t, rnd, be, pm)
	t.Logf("saved %d bytes", bytes)
}
