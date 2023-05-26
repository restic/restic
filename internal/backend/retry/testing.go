package retry

import "testing"

// TestFastRetries reduces the initial retry delay to 1 millisecond
func TestFastRetries(_ testing.TB) {
	fastRetries = true
}
