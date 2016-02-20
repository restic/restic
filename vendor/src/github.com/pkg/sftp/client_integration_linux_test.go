package sftp

import (
	"syscall"
	"testing"
)

const sftpServer = "/usr/lib/openssh/sftp-server"

func TestClientStatVFS(t *testing.T) {
	if *testServerImpl {
		t.Skipf("go server does not support FXP_EXTENDED")
	}
	sftp, cmd := testClient(t, READWRITE, NO_DELAY)
	defer cmd.Wait()
	defer sftp.Close()

	vfs, err := sftp.StatVFS("/")
	if err != nil {
		t.Fatal(err)
	}

	// get system stats
	s := syscall.Statfs_t{}
	err = syscall.Statfs("/", &s)
	if err != nil {
		t.Fatal(err)
	}

	// check some stats
	if vfs.Frsize != uint64(s.Frsize) {
		t.Fatal("fr_size does not match")
	}

	if vfs.Bsize != uint64(s.Bsize) {
		t.Fatal("f_bsize does not match")
	}

	if vfs.Namemax != uint64(s.Namelen) {
		t.Fatal("f_namemax does not match")
	}

	if vfs.Bavail != s.Bavail {
		t.Fatal("f_bavail does not match")
	}
}
