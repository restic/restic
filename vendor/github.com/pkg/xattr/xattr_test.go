// +build linux darwin freebsd

package xattr

import (
	"io/ioutil"
	"os"
	"testing"
)

const UserPrefix = "user."

func Test(t *testing.T) {
	tmp, err := ioutil.TempFile("", "")

	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())

	err = Set(tmp.Name(), UserPrefix+"test", []byte("test-attr-value"))
	if err != nil {
		t.Fatal(err)
	}

	list, err := List(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, name := range list {
		if name == UserPrefix+"test" {
			found = true
		}
	}

	if !found {
		t.Fatal("Listxattr did not return test attribute")
	}

	var data []byte
	data, err = Get(tmp.Name(), UserPrefix+"test")
	if err != nil {
		t.Fatal(err)
	}
	value := string(data)
	t.Log(value)
	if "test-attr-value" != value {
		t.Fail()
	}

	err = Remove(tmp.Name(), UserPrefix+"test")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNoData(t *testing.T) {
	tmp, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())

	err = Set(tmp.Name(), UserPrefix+"test", []byte{})
	if err != nil {
		t.Fatal(err)
	}

	list, err := List(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, name := range list {
		if name == UserPrefix+"test" {
			found = true
		}
	}

	if !found {
		t.Fatal("Listxattr did not return test attribute")
	}
}
