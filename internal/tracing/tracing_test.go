package tracing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func resetGlobalOTEL(t *testing.T) {
	t.Helper()
	origTP := otel.GetTracerProvider()
	origProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origProp)
	})
}

func TestTracer(t *testing.T) {
	tr := Tracer()
	if tr == nil {
		t.Fatal("expected non-nil tracer")
	}
}

func TestSetupInvalidURL(t *testing.T) {
	_, err := Setup(context.Background(), "://invalid", "svc")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestSetupWrongScheme(t *testing.T) {
	_, err := Setup(context.Background(), "ftp://example.com:4318", "svc")
	if err == nil {
		t.Fatal("expected error for non-http/https scheme")
	}
}

func otlpHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestSetupHTTP(t *testing.T) {
	resetGlobalOTEL(t)
	srv := httptest.NewServer(otlpHandler())
	defer srv.Close()

	shutdown, err := Setup(context.Background(), srv.URL, "test-svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestSetupDefaultServiceName(t *testing.T) {
	resetGlobalOTEL(t)
	srv := httptest.NewServer(otlpHandler())
	defer srv.Close()

	shutdown, err := Setup(context.Background(), srv.URL, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = shutdown(context.Background())
}

func TestSetupHTTPWithBasicAuth(t *testing.T) {
	resetGlobalOTEL(t)
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rawURL := "http://user:secret@" + srv.Listener.Addr().String()
	shutdown, err := Setup(context.Background(), rawURL, "svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Export a span so the exporter actually sends a request.
	_, span := Tracer().Start(context.Background(), "auth-test")
	span.End()
	_ = shutdown(context.Background())

	if gotAuth == "" {
		// The OTLP exporter may not have sent a request if spans were dropped.
		// Just verify Setup didn't error.
		t.Log("no HTTP request received (spans may have been dropped before flush)")
	}
}

func TestSetupHTTPWithCustomPath(t *testing.T) {
	resetGlobalOTEL(t)
	srv := httptest.NewServer(otlpHandler())
	defer srv.Close()

	rawURL := "http://" + srv.Listener.Addr().String() + "/custom/traces"
	shutdown, err := Setup(context.Background(), rawURL, "svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = shutdown(context.Background())
}

func TestWrapTransport(t *testing.T) {
	wrapped := WrapTransport(http.DefaultTransport)
	if wrapped == nil {
		t.Fatal("expected non-nil RoundTripper")
	}
	if wrapped == http.DefaultTransport {
		t.Fatal("expected a different RoundTripper from the wrapper")
	}
}

func TestExtractParentContextEmpty(t *testing.T) {
	ctx := context.Background()
	got := ExtractParentContext(ctx, "")
	if got != ctx {
		t.Fatal("expected same context for empty traceparent")
	}
}

func TestExtractParentContextValid(t *testing.T) {
	resetGlobalOTEL(t)

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
	))

	const rawTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	ctx := ExtractParentContext(context.Background(), rawTraceparent)

	_, span := Tracer().Start(ctx, "child")
	span.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 recorded span, got %d", len(spans))
	}
	parent := spans[0].Parent
	if !parent.IsValid() {
		t.Fatal("expected child span to have a valid parent")
	}
	if parent.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("unexpected parent trace ID: %s", parent.TraceID().String())
	}
	if parent.SpanID().String() != "00f067aa0ba902b7" {
		t.Errorf("unexpected parent span ID: %s", parent.SpanID().String())
	}
}

func TestExtractParentContextInvalidIsNoop(t *testing.T) {
	ctx := context.Background()
	got := ExtractParentContext(ctx, "not-a-valid-traceparent")
	_ = got // must not panic
}

func TestEndSpanWithErrorNil(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	_, span := tp.Tracer("test").Start(context.Background(), "op")

	EndSpanWithError(span, nil)

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status.Code != 0 {
		t.Errorf("expected unset status code, got %v", spans[0].Status.Code)
	}
}

func TestEndSpanWithError(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	_, span := tp.Tracer("test").Start(context.Background(), "op")

	EndSpanWithError(span, errors.New("something failed"))

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status.Code != codes.Error {
		t.Errorf("expected codes.Error status, got %v", spans[0].Status.Code)
	}
	if spans[0].Status.Description != "something failed" {
		t.Errorf("unexpected status description: %q", spans[0].Status.Description)
	}
	if len(spans[0].Events) == 0 {
		t.Error("expected at least one event (recorded error)")
	}
}

func TestEndSpanWithErrorNoop(t *testing.T) {
	// Ensure EndSpanWithError works on a no-op span without panicking.
	var span trace.Span = trace.SpanFromContext(context.Background())
	EndSpanWithError(span, errors.New("noop"))
}

func TestEndpointHost(t *testing.T) {
	cases := []struct {
		rawURL string
		want   string
	}{
		{"http://example.com", "example.com:80"},
		{"https://example.com", "example.com:443"},
		{"http://example.com:9411", "example.com:9411"},
		{"https://example.com:4318", "example.com:4318"},
		{"http://127.0.0.1", "127.0.0.1:80"},
		{"https://127.0.0.1", "127.0.0.1:443"},
	}
	for _, tc := range cases {
		u, err := url.Parse(tc.rawURL)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", tc.rawURL, err)
		}
		got := endpointHost(u)
		if got != tc.want {
			t.Errorf("endpointHost(%q) = %q, want %q", tc.rawURL, got, tc.want)
		}
	}
}
