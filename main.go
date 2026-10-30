package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const maxBodyBytes = 1 << 20

var (
	version = "dev"
	commit  = "unknown"
)

type versionInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	Go         string `json:"go"`
	PID        int    `json:"pid"`
	GOMAXPROCS int    `json:"gomaxprocs"`
}

func currentVersion() versionInfo {
	return versionInfo{
		Name: "textstats", Version: version, Commit: commit,
		Go: runtime.Version(), PID: os.Getpid(), GOMAXPROCS: runtime.GOMAXPROCS(0),
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(w, "request body exceeds 1 MiB", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "request must contain one JSON object with a text field", http.StatusBadRequest)
}

func handleAnalyze(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer r.Body.Close()

	var request struct {
		Text string `json:"text"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeDecodeError(w, err)
		return
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		writeDecodeError(w, err)
		return
	}
	if strings.TrimSpace(request.Text) == "" {
		http.Error(w, "text must not be empty", http.StatusBadRequest)
		return
	}
	writeJSON(w, Analyze(request.Text))
}

func newMux() http.Handler {
	monitor := newTelemetry()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, currentVersion())
	})
	mux.HandleFunc("POST /api/analyze", handleAnalyze)
	mux.Handle("GET /metrics", monitor.reg.Handler())
	return monitor.wrap(mux)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "HTTP listen address")
	showVersion := flag.Bool("version", false, "print build and runtime information")
	flag.Parse()
	if *showVersion {
		if err := json.NewEncoder(os.Stdout).Encode(currentVersion()); err != nil {
			log.Fatal(err)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{
		Addr: *addr, Handler: newMux(), ReadHeaderTimeout: 5 * time.Second,
	}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()
	log.Printf("textstats listening on %s; GOMAXPROCS=%d", *addr, runtime.GOMAXPROCS(0))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-stopped
}
