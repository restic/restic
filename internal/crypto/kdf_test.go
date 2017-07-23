package crypto

import (
	"testing"
	"time"
)

func TestCalibrate(t *testing.T) {
	params, err := Calibrate(100*time.Millisecond, 50)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("testing calibrate, params after: %v", params)
}
