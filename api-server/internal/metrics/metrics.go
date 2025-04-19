package metrics

import (
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Prometheus and Grafana friendly
var (
	ClaimsSubmitted *prometheus.CounterVec
	ClaimsFailed    *prometheus.CounterVec
)

func StartPrometheus(r *chi.Mux) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	ClaimsSubmitted = promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: "claims_submitted_total",
			Help: "Total number of claims submitted",
		},
		[]string{"region", "username"},
	)

	ClaimsFailed = promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: "claims_failed_total",
			Help: "Total number of failed claims",
		},
		[]string{"region", "username"},
	)

	r.Handle("/metrics", promhttp.Handler()) // expose Prometheus metrics
}
