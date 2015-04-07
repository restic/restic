package fuse

func localVolume(conf *MountConfig) error {
	return nil
}

func volumeName(name string) MountOption {
	return dummyOption
}
