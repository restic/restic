package backend

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCustomUserAgentTransport(t *testing.T) {
	// Create a mock HTTP handler that checks the User-Agent header
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent := r.Header.Get("User-Agent")
		if userAgent != "TestUserAgent" {
			t.Errorf("Expected User-Agent: TestUserAgent, got: %s", userAgent)
		}
		w.WriteHeader(http.StatusOK)
	})

	// Create a test server with the mock handler
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create a custom user agent transport
	customUserAgent := "TestUserAgent"
	transport := &httpUserAgentRoundTripper{
		userAgent: customUserAgent,
		rt:        http.DefaultTransport,
	}

	// Create an HTTP client with the custom transport
	client := &http.Client{
		Transport: transport,
	}

	// Make a request to the test server
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Log("failed to close response body")
		}
	}()

	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code: %d, got: %d", http.StatusOK, resp.StatusCode)
	}
}
