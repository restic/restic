package tracing

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationScope = "github.com/restic/restic"

// Tracer returns the global restic tracer. When Setup has not been called it
// returns a no-op tracer so callers need no conditional guards.
func Tracer() trace.Tracer {
	return otel.Tracer(instrumentationScope)
}

// Setup initialises the OpenTelemetry SDK with an OTLP/HTTP exporter.
//
// rawURL must use the http or https scheme. Basic-auth credentials may be
// embedded in the URL userinfo component, e.g.:
//
//	https://user:pass@collector.example.com:4318
//
// serviceName is used as the service.name resource attribute (default "restic").
// Returns a shutdown function that must be called to flush pending spans.
func Setup(ctx context.Context, rawURL, serviceName string) (shutdown func(context.Context) error, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid trace URL %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("trace URL must use http or https scheme, got %q", u.Scheme)
	}

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpointHost(u)),
	}
	if u.Scheme == "http" {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	if u.User != nil {
		user := u.User.Username()
		pass, _ := u.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		opts = append(opts, otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Basic " + token,
		}))
	}
	// Override the default /v1/traces path when the URL carries an explicit one.
	if u.Path != "" && u.Path != "/" {
		opts = append(opts, otlptracehttp.WithURLPath(u.Path))
	}

	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP trace exporter: %w", err)
	}

	if serviceName == "" {
		serviceName = "restic"
	}
	sysInfo := Collect()
	res, err := sdkresource.New(ctx,
		sdkresource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("host.name", sysInfo.FQDN),
		),
	)
	if err != nil || res == nil {
		res = sdkresource.Default()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// WrapTransport wraps rt with OpenTelemetry HTTP instrumentation so that every
// outgoing request carries a W3C traceparent header and is recorded as a child
// span of the currently active span.  When no tracer provider has been
// configured (--trace not supplied) the wrapper delegates to rt without
// overhead.
func WrapTransport(rt http.RoundTripper) http.RoundTripper {
	return otelhttp.NewTransport(rt)
}

// ExtractParentContext parses a W3C traceparent string and returns a context
// that carries it as the remote parent span context. If traceparent is empty
// the original context is returned unchanged.
func ExtractParentContext(ctx context.Context, traceparent string) context.Context {
	if traceparent == "" {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier{
		"traceparent": traceparent,
	})
}

// EndSpanWithError records err on the span (if non-nil) and calls End.
func EndSpanWithError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// endpointHost returns "host:port" from a parsed URL, defaulting to standard
// HTTP/HTTPS ports when none is explicitly specified.
func endpointHost(u *url.URL) string {
	if u.Port() != "" {
		return u.Hostname() + ":" + u.Port()
	}
	if u.Scheme == "https" {
		return u.Hostname() + ":443"
	}
	return u.Hostname() + ":80"
}
