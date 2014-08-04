package khepri

import "os"

type Object struct {
	repo *Repository

	id  ID
	tpe Type

	hw   HashingWriter
	file *os.File
}

func (repo *Repository) NewObject(t Type) (*Object, error) {
	obj := &Object{
		repo: repo,
		tpe:  t,
	}

	return obj, obj.open()
}

func (obj *Object) open() error {
	if obj.isFinal() {
		panic("object is finalized")
	}

	if obj.isOpen() {
		panic("object already open")
	}

	// create tempfile in repository
	if obj.hw == nil {
		// save contents to tempfile, hash while writing
		var err error
		obj.file, err = obj.repo.tempFile()
		if err != nil {
			return err
		}

		// create hashing writer
		obj.hw = NewHashingWriter(obj.file, obj.repo.hash)
	}

	return nil
}

func (obj *Object) isOpen() bool {
	return obj.file != nil && obj.hw != nil
}

func (obj *Object) isFinal() bool {
	return obj.id != nil
}

func (obj *Object) Write(data []byte) (int, error) {
	if !obj.isOpen() {
		panic("object not open")
	}

	return obj.hw.Write(data)
}

func (obj *Object) Close() error {
	if obj.file == nil || obj.hw == nil {
		panic("object is not open")
	}

	obj.file.Close()

	hash := obj.hw.Hash()

	// move file to final name using hash of contents
	id := ID(hash)
	err := obj.repo.renameFile(obj.file, obj.tpe, id)
	if err != nil {
		return err
	}

	obj.hw = nil
	obj.file = nil

	obj.id = id
	return nil
}

func (obj *Object) ID() ID {
	if !obj.isFinal() {
		panic("object not finalized")
	}

	return obj.id
}

func (obj *Object) Type() Type {
	return obj.tpe
}

func (obj *Object) Remove() error {
	if obj.id != nil {
		return obj.repo.Remove(obj.tpe, obj.id)
	}

	if obj.file != nil {
		file := obj.file
		obj.hw = nil
		obj.file = nil

		err := file.Close()
		if err != nil {
			return err
		}

		return os.Remove(file.Name())
	}

	return nil
}
