// This tests WatchdogReader

package swift

import (
	"io/ioutil"
	"testing"
	"time"
)

// Uses testReader from timeout_reader_test.go

func testWatchdogReaderTimeout(t *testing.T, initialTimeout, watchdogTimeout time.Duration, expectedTimeout bool) {
	test := newTestReader(3, 10*time.Millisecond)
	timer := time.NewTimer(initialTimeout)
	firedChan := make(chan bool)
	started := make(chan bool)
	go func() {
		started <- true
		select {
		case <-timer.C:
			firedChan <- true
		}
	}()
	<-started
	wr := newWatchdogReader(test, watchdogTimeout, timer)
	b, err := ioutil.ReadAll(wr)
	if err != nil || string(b) != "AAA" {
		t.Fatalf("Bad read %s %s", err, b)
	}
	fired := false
	select {
	case fired = <-firedChan:
	default:
	}
	if expectedTimeout {
		if !fired {
			t.Fatal("Timer should have fired")
		}
	} else {
		if fired {
			t.Fatal("Timer should not have fired")
		}
	}
}

func TestWatchdogReaderNoTimeout(t *testing.T) {
	testWatchdogReaderTimeout(t, 100*time.Millisecond, 100*time.Millisecond, false)
}

func TestWatchdogReaderTimeout(t *testing.T) {
	testWatchdogReaderTimeout(t, 5*time.Millisecond, 5*time.Millisecond, true)
}

func TestWatchdogReaderNoTimeoutShortInitial(t *testing.T) {
	testWatchdogReaderTimeout(t, 5*time.Millisecond, 100*time.Millisecond, false)
}

func TestWatchdogReaderTimeoutLongInitial(t *testing.T) {
	testWatchdogReaderTimeout(t, 100*time.Millisecond, 5*time.Millisecond, true)
}
