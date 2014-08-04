package khepri

import (
	"io"
	"os"
)

type createObject struct {
	repo *Repository

	tpe Type

	hw   HashingWriter
	file *os.File

	ch chan ID
}

func (repo *Repository) Create(t Type) (io.WriteCloser, <-chan ID, error) {
	obj := &createObject{
		repo: repo,
		tpe:  t,
		ch:   make(chan ID, 1),
	}

	// save contents to tempfile in repository, hash while writing
	var err error
	obj.file, err = obj.repo.tempFile()
	if err != nil {
		return nil, nil, err
	}

	// create hashing writer
	obj.hw = NewHashingWriter(obj.file, obj.repo.hash)

	return obj, obj.ch, nil
}

func (obj *createObject) Write(data []byte) (int, error) {
	if obj.hw == nil {
		panic("createObject: already closed!")
	}

	return obj.hw.Write(data)
}

func (obj *createObject) Close() error {
	if obj.hw == nil {
		panic("createObject: already closed!")
	}

	obj.file.Close()

	id := ID(obj.hw.Hash())
	obj.ch <- id

	// move file to final name using hash of contents
	err := obj.repo.renameFile(obj.file, obj.tpe, id)
	if err != nil {
		return err
	}

	obj.hw = nil
	obj.file = nil
	return nil
}
