//go:build !linux

package main

import (
	"runtime"

	"textstats/internal/metrics"
)

// The formal CPU experiment requires Linux. Do not report fabricated zero CPU
// usage when the application's functional demo runs on another platform.
func registerProcessMetrics(reg *metrics.Registry) {
	reg.NewGaugeFunc("textstats_process_metrics_supported",
		"Linux process metrics are available", func() float64 { return 0 })
	reg.NewGaugeFunc("go_goroutines", "Current goroutines",
		func() float64 { return float64(runtime.NumGoroutine()) })
	reg.NewGaugeFunc("go_gomaxprocs", "Go execution parallelism",
		func() float64 { return float64(runtime.GOMAXPROCS(0)) })
}
