package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	DeletedClaims = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nosqlclaims_deleted_total",
			Help: "Total number of expired NoSQLClaims deleted",
		},
		[]string{},
	)

	SkippedClaims = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nosqlclaims_skipped_total",
			Help: "Total number of NoSQLClaims skipped because TTL not met",
		},
		[]string{},
	)

	ReconcileDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "nosqlclaim_reconcile_duration_seconds",
			Help:    "Duration of NoSQLClaim reconciliation",
			Buckets: prometheus.DefBuckets,
		},
	)
)

func RegisterMetrics() {
	prometheus.MustRegister(DeletedClaims, SkippedClaims, ReconcileDuration)
}
