package fuse

func localVolume(conf *mountConfig) error {
	conf.options["local"] = ""
	return nil
}

func volumeName(name string) MountOption {
	return func(conf *mountConfig) error {
		conf.options["volname"] = name
		return nil
	}
}
