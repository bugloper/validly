// Package observability wires up structured logging, OpenTelemetry metrics
// (exported in Prometheus format) and OpenTelemetry tracing (exported over
// OTLP/HTTP when the standard OTEL_EXPORTER_OTLP_* variables are set).
package observability

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/validly/validly/internal/dns"
)

const instrumentation = "github.com/validly/validly"

// NewLogger returns a JSON slog logger at the given level ("debug", "info",
// "warn", "error").
func NewLogger(w io.Writer, level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: l}))
}

// Telemetry holds the metric instruments and tracer.
type Telemetry struct {
	Tracer         trace.Tracer
	MetricsHandler http.Handler
	Metrics        *Metrics
	shutdown       []func(context.Context) error
}

// Setup configures metrics and tracing for the service.
func Setup(ctx context.Context, version string) (*Telemetry, error) {
	res := resource.NewSchemaless(
		semconv.ServiceName("validly"),
		semconv.ServiceVersion(version),
	)

	reg := promclient.NewRegistry()
	exporter, err := otelprom.New(otelprom.WithRegisterer(reg))
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter), sdkmetric.WithResource(res))
	metrics, err := NewMetrics(mp.Meter(instrumentation))
	if err != nil {
		return nil, err
	}

	t := &Telemetry{
		MetricsHandler: promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
		Metrics:        metrics,
		shutdown:       []func(context.Context) error{mp.Shutdown},
	}

	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" {
		exp, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, err
		}
		tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp), sdktrace.WithResource(res))
		otel.SetTracerProvider(tp)
		t.Tracer = tp.Tracer(instrumentation)
		t.shutdown = append(t.shutdown, tp.Shutdown)
	} else {
		t.Tracer = noop.NewTracerProvider().Tracer(instrumentation)
	}
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return t, nil
}

// Shutdown flushes exporters.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	var errs []error
	for _, f := range t.shutdown {
		errs = append(errs, f(ctx))
	}
	return errors.Join(errs...)
}

// Metrics records Validly's service metrics. It implements dns.Observer and
// verification.Observer.
type Metrics struct {
	httpRequests  metric.Int64Counter
	httpDuration  metric.Float64Histogram
	validationDur metric.Float64Histogram
	dnsLookups    metric.Int64Counter
	dnsDuration   metric.Float64Histogram
	cacheRequests metric.Int64Counter
	apiErrors     metric.Int64Counter
	verifications metric.Int64Counter
}

// NewMetrics creates the instruments on meter.
func NewMetrics(meter metric.Meter) (*Metrics, error) {
	var (
		m    Metrics
		errs []error
		err  error
	)
	buckets := metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5)
	m.httpRequests, err = meter.Int64Counter("validly.http.requests", metric.WithDescription("HTTP requests served."))
	errs = append(errs, err)
	m.httpDuration, err = meter.Float64Histogram("validly.http.request.duration", metric.WithUnit("s"), buckets)
	errs = append(errs, err)
	m.validationDur, err = meter.Float64Histogram("validly.validation.duration", metric.WithUnit("s"), buckets,
		metric.WithDescription("Time to validate one address, including DNS."))
	errs = append(errs, err)
	m.dnsLookups, err = meter.Int64Counter("validly.dns.lookups", metric.WithDescription("Domain lookups by outcome (cache misses only)."))
	errs = append(errs, err)
	m.dnsDuration, err = meter.Float64Histogram("validly.dns.lookup.duration", metric.WithUnit("s"), buckets)
	errs = append(errs, err)
	m.cacheRequests, err = meter.Int64Counter("validly.cache.requests", metric.WithDescription("Cache lookups by result."))
	errs = append(errs, err)
	m.apiErrors, err = meter.Int64Counter("validly.api.errors", metric.WithDescription("API error responses by code."))
	errs = append(errs, err)
	m.verifications, err = meter.Int64Counter("validly.verification.events", metric.WithDescription("Ownership-verification events."))
	errs = append(errs, err)
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Metrics) HTTPRequest(route, method string, status int, d time.Duration) {
	attrs := metric.WithAttributes(
		attribute.String("http.route", route),
		attribute.String("http.request.method", method),
		attribute.Int("http.response.status_code", status),
	)
	m.httpRequests.Add(context.Background(), 1, attrs)
	m.httpDuration.Record(context.Background(), d.Seconds(), attrs)
}

func (m *Metrics) Validation(overall string, d time.Duration) {
	m.validationDur.Record(context.Background(), d.Seconds(), metric.WithAttributes(attribute.String("overall_status", overall)))
}

func (m *Metrics) APIError(code string) {
	m.apiErrors.Add(context.Background(), 1, metric.WithAttributes(attribute.String("code", code)))
}

func (m *Metrics) DNSLookup(status dns.Status, reason dns.Reason, d time.Duration) {
	attrs := metric.WithAttributes(attribute.String("status", string(status)), attribute.String("reason", string(reason)))
	m.dnsLookups.Add(context.Background(), 1, attrs)
	m.dnsDuration.Record(context.Background(), d.Seconds(), attrs)
}

func (m *Metrics) CacheResult(name string, hit bool) {
	result := "miss"
	if hit {
		result = "hit"
	}
	m.cacheRequests.Add(context.Background(), 1, metric.WithAttributes(attribute.String("cache", name), attribute.String("result", result)))
}

func (m *Metrics) VerificationEvent(event string) {
	m.verifications.Add(context.Background(), 1, metric.WithAttributes(attribute.String("event", strings.ToLower(event))))
}
