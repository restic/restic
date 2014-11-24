package backend_test

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/fd0/khepri/backend"
)

// assert fails the test if the condition is false.
func assert(tb testing.TB, condition bool, msg string, v ...interface{}) {
	if !condition {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: "+msg+"\033[39m\n\n", append([]interface{}{filepath.Base(file), line}, v...)...)
		tb.FailNow()
	}
}

// ok fails the test if an err is not nil.
func ok(tb testing.TB, err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: unexpected error: %s\033[39m\n\n", filepath.Base(file), line, err.Error())
		tb.FailNow()
	}
}

// equals fails the test if exp is not equal to act.
func equals(tb testing.TB, exp, act interface{}) {
	if !reflect.DeepEqual(exp, act) {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d:\n\n\texp: %#v\n\n\tgot: %#v\033[39m\n\n", filepath.Base(file), line, exp, act)
		tb.FailNow()
	}
}

func str2id(s string) backend.ID {
	id, err := backend.ParseID(s)
	if err != nil {
		panic(err)
	}

	return id
}

type IDList backend.IDs

var samples = IDList{
	str2id("20bdc1402a6fc9b633aaffffffffffffffffffffffffffffffffffffffffffff"),
	str2id("20bdc1402a6fc9b633ccd578c4a92d0f4ef1a457fa2e16c596bc73fb409d6cc0"),
	str2id("20bdc1402a6fc9b633ffffffffffffffffffffffffffffffffffffffffffffff"),
	str2id("20ff988befa5fc40350f00d531a767606efefe242c837aaccb80673f286be53d"),
	str2id("326cb59dfe802304f96ee9b5b9af93bdee73a30f53981e5ec579aedb6f1d0f07"),
	str2id("86b60b9594d1d429c4aa98fa9562082cabf53b98c7dc083abe5dae31074dd15a"),
	str2id("96c8dbe225079e624b5ce509f5bd817d1453cd0a85d30d536d01b64a8669aeae"),
	str2id("fa31d65b87affcd167b119e9d3d2a27b8236ca4836cb077ed3e96fcbe209b792"),
}

func (l IDList) List(backend.Type) (backend.IDs, error) {
	return backend.IDs(l), nil
}

func TestPrefixLength(t *testing.T) {
	l, err := backend.PrefixLength(samples, backend.Snapshot)
	ok(t, err)
	equals(t, 10, l)

	l, err = backend.PrefixLength(samples[:3], backend.Snapshot)
	ok(t, err)
	equals(t, 10, l)

	l, err = backend.PrefixLength(samples[3:], backend.Snapshot)
	ok(t, err)
	equals(t, 4, l)
}
