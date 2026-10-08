package middleware

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofault/gofault/core"
	"github.com/prometheus/client_golang/prometheus"
)

// MetricsConfig holds configuration for the metrics middleware.
type MetricsConfig struct {
	// Enabled enables the metrics middleware.
	Enabled bool
	// Namespace prefixes all metric names.
	Namespace string
	// Subsystem further qualifies the metric.
	Subsystem string
	// SkipHealthCheck excludes the /health path from metrics.
	SkipHealthCheck bool
	// Buckets for the request duration histogram.
	Buckets []float64
}

// DefaultMetricsConfig returns a default metrics configuration.
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Enabled:         true,
		Namespace:       "gofault",
		Subsystem:       "http",
		SkipHealthCheck: true,
		Buckets:         []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}
}

// httpMetrics holds all Prometheus metrics for a given namespace/subsystem.
type httpMetrics struct {
	requestsTotal    *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	requestsInFlight prometheus.Gauge
}

// newHttpMetrics creates metrics and registers them with the provided registry.
func newHttpMetrics(namespace, subsystem string, buckets []float64, registry *prometheus.Registry) httpMetrics {
	if len(buckets) == 0 {
		buckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
	}

	requestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "requests_total",
			Help:      "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)
	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			Buckets:   buckets,
		},
		[]string{"method", "path", "status"},
	)
	requestsInFlight := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "requests_in_flight",
			Help:      "Number of HTTP requests currently being processed.",
		},
	)

	registry.MustRegister(requestsTotal, requestDuration, requestsInFlight)

	return httpMetrics{
		requestsTotal:    requestsTotal,
		requestDuration:  requestDuration,
		requestsInFlight: requestsInFlight,
	}
}

// MetricsMiddleware creates a middleware that records HTTP request metrics.
// Returns the middleware and the registry that holds the metrics.
func MetricsMiddleware(config MetricsConfig) (core.MiddlewareFunc, *prometheus.Registry) {
	if !config.Enabled {
		return nil, nil
	}

	namespace := config.Namespace
	if namespace == "" {
		namespace = "gofault"
	}
	subsystem := config.Subsystem
	if subsystem == "" {
		subsystem = "http"
	}

	registry := prometheus.NewRegistry()
	m := newHttpMetrics(namespace, subsystem, config.Buckets, registry)

	return func(ctx *core.Ctx, next core.Handler) error {
		// Skip health check endpoint to avoid noise
		if config.SkipHealthCheck && ctx.Request.URL.Path == "/health" {
			return next(ctx)
		}

		path := normalizePath(ctx.Request.URL.Path)
		method := ctx.Request.Method

		m.requestsInFlight.Inc()
		start := time.Now()

		err := next(ctx)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(ctx.StatusCode)
		if ctx.StatusCode == 0 {
			status = "0"
		}

		m.requestsTotal.WithLabelValues(method, path, status).Inc()
		m.requestDuration.WithLabelValues(method, path, status).Observe(duration)
		m.requestsInFlight.Dec()

		return err
	}, registry
}

// normalizePath returns a label-safe path.
// Dynamic segments like /user/123 are normalized to /user/:id to prevent
// Prometheus cardinality explosion from unbounded label values.
func normalizePath(path string) string {
	if path == "" {
		return "/"
	}

	// Split into segments and replace numeric ones with a placeholder.
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		// Replace purely numeric segments (IDs) with a placeholder.
		if isNumeric(seg) {
			segments[i] = ":id"
		}
	}

	return strings.Join(segments, "/")
}

// isNumeric reports whether s consists only of digits.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
