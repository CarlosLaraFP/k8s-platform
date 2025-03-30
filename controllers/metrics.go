package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	UpdatedClaims = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "claims_updated_total",
			Help: "Total number of new Claims updated",
		},
		[]string{},
	)

	DeletedClaims = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "claims_deleted_total",
			Help: "Total number of expired Claims deleted",
		},
		[]string{},
	)

	SkippedClaims = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "claims_skipped_total",
			Help: "Total number of Claims skipped because TTL not met",
		},
		[]string{},
	)

	ReconcileDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "claim_reconcile_duration_seconds",
			Help:    "Duration of Claim reconciliation",
			Buckets: prometheus.DefBuckets,
		},
	)
)

func RegisterMetrics() {
	prometheus.MustRegister(DeletedClaims, SkippedClaims, ReconcileDuration)
}
