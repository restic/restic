package tracing

import (
	"testing"
)

func TestCollect(t *testing.T) {
	info := Collect()
	if info.User == "" {
		t.Error("expected non-empty User")
	}
	if info.FQDN == "" {
		t.Error("expected non-empty FQDN")
	}
}

func TestCollectUserID(t *testing.T) {
	info := Collect()
	if info.UserID == "" {
		t.Error("expected non-empty UserID (numeric UID)")
	}
}

func TestResolveFQDNFallsBackToHostname(t *testing.T) {
	// An obviously non-routable hostname should fall back to the input.
	got := resolveFQDN("this-hostname-does-not-exist.invalid")
	if got != "this-hostname-does-not-exist.invalid" {
		t.Errorf("expected fallback to input, got %q", got)
	}
}

func TestResolveFQDNLocalhost(t *testing.T) {
	// "localhost" always resolves; result must be non-empty.
	got := resolveFQDN("localhost")
	if got == "" {
		t.Error("expected non-empty result for 'localhost'")
	}
}
