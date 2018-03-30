package backoff

import (
	"math/rand"
	"testing"
	"time"
)

func TestMaxTriesHappy(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	max := 17 + r.Intn(13)
	bo := WithMaxRetries(&ZeroBackOff{}, uint64(max))

	// Load up the tries count, but reset should clear the record
	for ix := 0; ix < max/2; ix++ {
		bo.NextBackOff()
	}
	bo.Reset()

	// Now fill the tries count all the way up
	for ix := 0; ix < max; ix++ {
		d := bo.NextBackOff()
		if d == Stop {
			t.Errorf("returned Stop on try %d", ix)
		}
	}

	// We have now called the BackOff max number of times, we expect
	// the next result to be Stop, even if we try it multiple times
	for ix := 0; ix < 7; ix++ {
		d := bo.NextBackOff()
		if d != Stop {
			t.Error("invalid next back off")
		}
	}

	// Reset makes it all work again
	bo.Reset()
	d := bo.NextBackOff()
	if d == Stop {
		t.Error("returned Stop after reset")
	}

}

func TestMaxTriesZero(t *testing.T) {
	// It might not make sense, but its okay to send a zero
	bo := WithMaxRetries(&ZeroBackOff{}, uint64(0))
	for ix := 0; ix < 11; ix++ {
		d := bo.NextBackOff()
		if d == Stop {
			t.Errorf("returned Stop on try %d", ix)
		}
	}
}
