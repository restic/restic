package khepri

func (repo *Repository) Create(t Type, data []byte) (ID, error) {
	// TODO: make sure that tempfile is removed upon error

	// create tempfile in repository
	var err error
	file, err := repo.tempFile()
	if err != nil {
		return nil, err
	}

	// write data to tempfile
	_, err = file.Write(data)
	if err != nil {
		return nil, err
	}

	// close tempfile, return id
	id := IDFromData(data)
	err = repo.renameFile(file, t, id)
	if err != nil {
		return nil, err
	}

	return id, nil
}
