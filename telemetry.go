package main

import (
	"net/http"
	"strconv"
	"time"

	"textstats/internal/metrics"
)

const analyzeRoute = "/api/analyze"

type telemetry struct {
	reg      *metrics.Registry
	requests *metrics.CounterVec
	duration *metrics.HistogramVec
	inflight *metrics.Gauge
}

func newTelemetry() *telemetry {
	reg := metrics.NewRegistry()
	t := &telemetry{
		reg: reg,
		requests: reg.NewCounterVec("http_requests_total",
			"Completed business requests by route, method and response status", "route", "method", "code"),
		duration: reg.NewHistogramVec("http_request_duration_seconds",
			"Server handler duration, excluding network and pre-handler queueing",
			metrics.LatencyBuckets, "route", "method", "code"),
		inflight: reg.NewGaugeVec("http_inflight_requests",
			"Business requests currently executing", "route").With(analyzeRoute),
	}
	// Make the successful-request series visible before the first request.
	t.requests.With(analyzeRoute, http.MethodPost, "200")
	t.duration.With(analyzeRoute, http.MethodPost, "200")
	info := currentVersion()
	reg.NewGaugeVec("textstats_info", "Build information", "version", "commit", "go_version").
		With(info.Version, info.Commit, info.Go).Set(1)
	registerProcessMetrics(reg)
	return t
}

// wrap measures only the business path. Scrapes, health checks, version requests
// and unknown paths do not inflate business traffic. Method errors on the
// business path are still observed as 405 responses.
func (t *telemetry) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != analyzeRoute {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		t.inflight.Inc()
		defer t.inflight.Dec()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		elapsed := time.Since(start).Seconds()
		code := strconv.Itoa(rec.status())
		method := metricMethod(r.Method)
		t.duration.With(analyzeRoute, method, code).Observe(elapsed)
		t.requests.With(analyzeRoute, method, code).Inc()
	})
}

func metricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodDelete, http.MethodConnect, http.MethodOptions,
		http.MethodTrace, http.MethodPatch:
		return method
	default:
		return "OTHER"
	}
}

// statusRecorder follows net/http's first-final-status and implicit-200 rules.
// Unwrap allows http.ResponseController to reach the original writer.
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
		r.ResponseWriter.WriteHeader(code)
		return
	}
	if r.code != 0 {
		return
	}
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if r.code == 0 {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(body)
}

func (r *statusRecorder) status() int {
	if r.code == 0 {
		return http.StatusOK
	}
	return r.code
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
