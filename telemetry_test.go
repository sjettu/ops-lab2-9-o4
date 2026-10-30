package main

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func scrapeSamples(t *testing.T, handler http.Handler) map[string]float64 {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK ||
		!strings.HasPrefix(response.Header().Get("Content-Type"), "text/plain; version=0.0.4") {
		t.Fatalf("invalid metrics response: status=%d, type=%q",
			response.Code, response.Header().Get("Content-Type"))
	}
	samples := make(map[string]float64)
	for _, line := range strings.Split(response.Body.String(), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("invalid metric line %q", line)
		}
		value, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			t.Fatalf("invalid metric value %q: %v", line, err)
		}
		if _, exists := samples[fields[0]]; exists {
			t.Fatalf("duplicate metric series %q", fields[0])
		}
		samples[fields[0]] = value
	}
	return samples
}

func requestKey(name, method, code string) string {
	return fmt.Sprintf(`%s{route="/api/analyze",method=%q,code=%q}`, name, method, code)
}

func businessRequest(handler http.Handler, method, body string) int {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response,
		httptest.NewRequest(method, analyzeRoute, strings.NewReader(body)))
	return response.Code
}

func TestMetricsCountBusinessAndExcludeScrapes(t *testing.T) {
	handler := newMux()
	if businessRequest(handler, http.MethodPost, `{"text":"hello world"}`) != 200 ||
		businessRequest(handler, http.MethodPost, `{"text":`) != 400 ||
		businessRequest(handler, http.MethodGet, "") != 405 {
		t.Fatal("unexpected business response")
	}
	for _, path := range []string{"/healthz", "/version", "/unknown", "/metrics", "/metrics"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	samples := scrapeSamples(t, handler)
	var total float64
	for name, value := range samples {
		if strings.HasPrefix(name, "http_requests_total{") {
			total += value
		}
	}
	if total != 3 {
		t.Fatalf("scrapes or non-business routes counted: total=%g", total)
	}
	for _, labels := range [][2]string{{"POST", "200"}, {"POST", "400"}, {"GET", "405"}} {
		for _, metric := range []string{"http_requests_total", "http_request_duration_seconds_count"} {
			key := requestKey(metric, labels[0], labels[1])
			if samples[key] != 1 {
				t.Fatalf("%s=%g, want 1", key, samples[key])
			}
		}
	}
	if samples[`http_inflight_requests{route="/api/analyze"}`] != 0 {
		t.Fatal("inflight did not return to zero")
	}
}

func TestMetricsCaptureServerErrorsAndFirstStatus(t *testing.T) {
	for _, test := range []struct {
		name   string
		first  int
		second int
	}{
		{"server error", 500, 0},
		{"first status wins", 201, 500},
		{"implicit success", 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			monitor := newTelemetry()
			handler := monitor.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.first != 0 {
					w.WriteHeader(test.first)
				}
				if test.second != 0 {
					w.WriteHeader(test.second)
				}
				_, _ = w.Write([]byte("response"))
			}))
			status := businessRequest(handler, "POST", `{"text":"hi"}`)
			want := test.first
			if want == 0 {
				want = 200
			}
			if status != want {
				t.Fatalf("response status=%d, want=%d", status, want)
			}
			samples := scrapeSamples(t, monitor.reg.Handler())
			if samples[requestKey("http_requests_total", "POST", strconv.Itoa(want))] != 1 {
				t.Fatal("response status not reflected in counter")
			}
			if test.first == 201 && samples[requestKey("http_requests_total", "POST", "500")] != 0 {
				t.Fatal("late WriteHeader incorrectly counted as 500")
			}
		})
	}
}

func TestSLO20msBucketIncludesBoundary(t *testing.T) {
	monitor := newTelemetry()
	monitor.duration.With(analyzeRoute, "POST", "200").Observe(0.02)
	samples := scrapeSamples(t, monitor.reg.Handler())
	prefix := `http_request_duration_seconds_bucket{route="/api/analyze",method="POST",code="200",le="`
	if samples[prefix+`0.01"}`] != 0 ||
		samples[prefix+`0.02"}`] != 1 ||
		samples[prefix+`+Inf"}`] != 1 {
		t.Fatal("20 ms boundary not represented by the exact cumulative bucket")
	}
}

func TestInflightVisibleDuringRequest(t *testing.T) {
	monitor := newTelemetry()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	handler := monitor.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	go func() {
		businessRequest(handler, "POST", "")
		close(done)
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("business handler not reached")
	}
	if scrapeSamples(t, monitor.reg.Handler())[`http_inflight_requests{route="/api/analyze"}`] != 1 {
		t.Fatal("running request not visible")
	}
	unblock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("business request did not finish")
	}
	if scrapeSamples(t, monitor.reg.Handler())[`http_inflight_requests{route="/api/analyze"}`] != 0 {
		t.Fatal("completed request remains inflight")
	}
}

func TestConcurrentRequestsAndScrapes(t *testing.T) {
	const requests, scrapes = 80, 20
	handler := newMux()
	start := make(chan struct{})
	failures := make(chan string, requests+scrapes)
	var group sync.WaitGroup
	for i := 0; i < requests; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			if businessRequest(handler, "POST", `{"text":"one two two"}`) != 200 {
				failures <- "business request failed"
			}
		}()
	}
	for i := 0; i < scrapes; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
			if response.Code != 200 {
				failures <- "scrape failed"
			}
		}()
	}
	close(start)
	group.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
	samples := scrapeSamples(t, handler)
	if samples[requestKey("http_requests_total", "POST", "200")] != requests ||
		samples[requestKey("http_request_duration_seconds_count", "POST", "200")] != requests {
		t.Fatal("concurrent requests lost or scrapes counted")
	}
	if samples[`http_inflight_requests{route="/api/analyze"}`] != 0 {
		t.Fatal("inflight leaked after concurrent requests")
	}
}

func TestProcessMetrics(t *testing.T) {
	monitor := newTelemetry()
	first := scrapeSamples(t, monitor.reg.Handler())
	if first["go_gomaxprocs"] < 1 || first["go_goroutines"] < 1 {
		t.Fatal("missing Go runtime metrics")
	}
	if runtime.GOOS != "linux" {
		if first["textstats_process_metrics_supported"] != 0 {
			t.Fatal("unsupported process metrics must be marked explicitly")
		}
		return
	}
	cpu, exists := first["process_cpu_seconds_total"]
	if !exists || cpu < 0 || math.IsNaN(cpu) || math.IsInf(cpu, 0) ||
		first["textstats_process_metrics_supported"] != 1 {
		t.Fatal("invalid Linux CPU counter")
	}
	for i := 0; i < 30; i++ {
		if Analyze(strings.Repeat("one two three ", 1000)).WordCount != 3000 {
			t.Fatal("unexpected CPU workload result")
		}
	}
	second := scrapeSamples(t, monitor.reg.Handler())
	if second["process_cpu_seconds_total"] < cpu {
		t.Fatal("process CPU counter decreased")
	}
}
