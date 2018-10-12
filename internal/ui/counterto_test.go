package ui

import (
	"testing"
	"time"

	rtest "github.com/restic/restic/internal/test"
)

func TestETABasic(t *testing.T) {
	start := time.Now()

	c := StartCountTo(start, 100)

	c.Add(10)
	eta := c.ETA(start.Add(10 * time.Second))
	rtest.Equals(t, 90*time.Second, eta)

	c.Add(80)
	eta = c.ETA(start.Add(90 * time.Second))
	rtest.Equals(t, time.Second*10, eta)
}
