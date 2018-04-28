package tomb_test

import (
	"errors"
	"gopkg.in/tomb.v2"
	"reflect"
	"testing"
)

func nothing() error { return nil }

func TestNewTomb(t *testing.T) {
	tb := &tomb.Tomb{}
	checkState(t, tb, false, false, tomb.ErrStillAlive)
}

func TestGo(t *testing.T) {
	tb := &tomb.Tomb{}
	alive := make(chan bool)
	tb.Go(func() error {
		alive <- true
		tb.Go(func() error {
			alive <- true
			<-tb.Dying()
			return nil
		})
		<-tb.Dying()
		return nil
	})
	<-alive
	<-alive
	checkState(t, tb, false, false, tomb.ErrStillAlive)
	tb.Kill(nil)
	tb.Wait()
	checkState(t, tb, true, true, nil)
}

func TestGoErr(t *testing.T) {
	first := errors.New("first error")
	second := errors.New("first error")
	tb := &tomb.Tomb{}
	alive := make(chan bool)
	tb.Go(func() error {
		alive <- true
		tb.Go(func() error {
			alive <- true
			return first
		})
		<-tb.Dying()
		return second
	})
	<-alive
	<-alive
	tb.Wait()
	checkState(t, tb, true, true, first)
}

func TestGoPanic(t *testing.T) {
	// ErrDying being used properly, after a clean death.
	tb := &tomb.Tomb{}
	tb.Go(nothing)
	tb.Wait()
	defer func() {
		err := recover()
		if err != "tomb.Go called after all goroutines terminated" {
			t.Fatalf("Wrong panic on post-death tomb.Go call: %v", err)
		}
		checkState(t, tb, true, true, nil)
	}()
	tb.Go(nothing)
}

func TestKill(t *testing.T) {
	// a nil reason flags the goroutine as dying
	tb := &tomb.Tomb{}
	tb.Kill(nil)
	checkState(t, tb, true, false, nil)

	// a non-nil reason now will override Kill
	err := errors.New("some error")
	tb.Kill(err)
	checkState(t, tb, true, false, err)

	// another non-nil reason won't replace the first one
	tb.Kill(errors.New("ignore me"))
	checkState(t, tb, true, false, err)

	tb.Go(nothing)
	tb.Wait()
	checkState(t, tb, true, true, err)
}

func TestKillf(t *testing.T) {
	tb := &tomb.Tomb{}

	err := tb.Killf("BO%s", "OM")
	if s := err.Error(); s != "BOOM" {
		t.Fatalf(`Killf("BO%s", "OM"): want "BOOM", got %q`, s)
	}
	checkState(t, tb, true, false, err)

	// another non-nil reason won't replace the first one
	tb.Killf("ignore me")
	checkState(t, tb, true, false, err)

	tb.Go(nothing)
	tb.Wait()
	checkState(t, tb, true, true, err)
}

func TestErrDying(t *testing.T) {
	// ErrDying being used properly, after a clean death.
	tb := &tomb.Tomb{}
	tb.Kill(nil)
	tb.Kill(tomb.ErrDying)
	checkState(t, tb, true, false, nil)

	// ErrDying being used properly, after an errorful death.
	err := errors.New("some error")
	tb.Kill(err)
	tb.Kill(tomb.ErrDying)
	checkState(t, tb, true, false, err)

	// ErrDying being used badly, with an alive tomb.
	tb = &tomb.Tomb{}
	defer func() {
		err := recover()
		if err != "tomb: Kill with ErrDying while still alive" {
			t.Fatalf("Wrong panic on Kill(ErrDying): %v", err)
		}
		checkState(t, tb, false, false, tomb.ErrStillAlive)
	}()
	tb.Kill(tomb.ErrDying)
}

func TestKillErrStillAlivePanic(t *testing.T) {
	tb := &tomb.Tomb{}
	defer func() {
		err := recover()
		if err != "tomb: Kill with ErrStillAlive" {
			t.Fatalf("Wrong panic on Kill(ErrStillAlive): %v", err)
		}
		checkState(t, tb, false, false, tomb.ErrStillAlive)
	}()
	tb.Kill(tomb.ErrStillAlive)
}

func checkState(t *testing.T, tb *tomb.Tomb, wantDying, wantDead bool, wantErr error) {
	select {
	case <-tb.Dying():
		if !wantDying {
			t.Error("<-Dying: should block")
		}
	default:
		if wantDying {
			t.Error("<-Dying: should not block")
		}
	}
	seemsDead := false
	select {
	case <-tb.Dead():
		if !wantDead {
			t.Error("<-Dead: should block")
		}
		seemsDead = true
	default:
		if wantDead {
			t.Error("<-Dead: should not block")
		}
	}
	if err := tb.Err(); err != wantErr {
		t.Errorf("Err: want %#v, got %#v", wantErr, err)
	}
	if wantDead && seemsDead {
		waitErr := tb.Wait()
		switch {
		case waitErr == tomb.ErrStillAlive:
			t.Errorf("Wait should not return ErrStillAlive")
		case !reflect.DeepEqual(waitErr, wantErr):
			t.Errorf("Wait: want %#v, got %#v", wantErr, waitErr)
		}
	}
}
