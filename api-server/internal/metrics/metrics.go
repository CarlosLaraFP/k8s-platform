package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus and Grafana friendly
type Metrics struct {
	Registry        *prometheus.Registry
	ClaimsSubmitted *prometheus.CounterVec
	ClaimsFailed    *prometheus.CounterVec
	ClaimLatency    *prometheus.HistogramVec
	Uptime          prometheus.Gauge
}

func InitPrometheus() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		Registry: reg,
		ClaimsSubmitted: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "claims_submitted_total",
				Help: "Total number of claims submitted",
			},
			[]string{"region", "username"},
		),
		ClaimsFailed: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "claims_failed_total",
				Help: "Total number of failed claims",
			},
			[]string{"region", "username"},
		),
		ClaimLatency: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "claim_submission_seconds",
				Help:    "Latency for submitting a claim",
				Buckets: prometheus.DefBuckets, // reasonable defaults
			},
			[]string{"region", "username"},
		),
		Uptime: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "control_plane_uptime_seconds",
				Help: "How long the control plane has been up",
			},
		),
	}

	go func() {
		start := time.Now()
		for {
			m.Uptime.Set(time.Since(start).Seconds())
			time.Sleep(5 * time.Second)
		}
	}()

	return m
}
