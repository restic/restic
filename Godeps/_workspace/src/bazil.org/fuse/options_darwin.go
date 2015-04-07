package fuse

func localVolume(conf *MountConfig) error {
	conf.options["local"] = ""
	return nil
}

func volumeName(name string) MountOption {
	return func(conf *MountConfig) error {
		conf.options["volname"] = name
		return nil
	}
}
