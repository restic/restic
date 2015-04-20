package arrar

// Functions in this file are used in the tests and the names of the functions
// along with the file and line numbers are used, so please don't mess around
// with them too much.

import (
	"errors"
)

func one() error {
	return errors.New("one")
}

func two() error {
	return Annotate(one(), "two")
}

func three() error {
	return Annotate(two(), "three")
}

func transtwo() error {
	return Translate(one(), errors.New("translated"), "transtwo")
}

func transthree() error {
	return Translate(two(), errors.New("translated"), "transthree")
}

func four() error {
	return Annotate(transthree(), "four")
}

func test_new() error {
	return New("get location")
}

type receiver struct{}

func (*receiver) Func() error {
	return New("method")
}

func method() error {
	obj := &receiver{}
	return obj.Func()
}
