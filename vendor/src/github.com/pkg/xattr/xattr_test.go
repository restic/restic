// +build linux darwin freebsd

package xattr

import (
	"io/ioutil"
	"os"
	"testing"
)

const UserPrefix = "user."

func Test_setxattr(t *testing.T) {
	tmp, err := ioutil.TempFile("", "")

	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())

	err = Setxattr(tmp.Name(), UserPrefix+"test", []byte("test-attr-value"))
	if err != nil {
		t.Fatal(err)
	}

	var data []byte
	data, err = Getxattr(tmp.Name(), UserPrefix+"test")
	if err != nil {
		t.Fatal(err)
	}
	value := string(data)
	t.Log(value)
	if "test-attr-value" != value {
		t.Fail()
	}
}
