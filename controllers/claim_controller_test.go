package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ✅ Define a minimal fake struct that mimics your NoSQLClaim CRD
type FakeNoSQLClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              map[string]string `json:"spec,omitempty"`
}

func (f *FakeNoSQLClaim) DeepCopyObject() runtime.Object {
	copy := *f
	return &copy
}

var fakeGVK = schema.GroupVersionKind{
	Group:   "platform.example.org",
	Version: "v1alpha1",
	Kind:    "NoSQLClaim",
}

func newTestClaim(name string, creationTime time.Time) *FakeNoSQLClaim {
	return &FakeNoSQLClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NoSQLClaim",
			APIVersion: "platform.example.org/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(creationTime),
		},
	}
}

func TestNoSQLClaimReconciler_Reconcile(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		claims        []client.Object
		ttl           int64
		expectDeleted bool
		expectSkipped bool
	}{
		{
			name:          "deletes expired claim",
			claims:        []client.Object{newTestClaim("expired", now.Add(-4*time.Hour))},
			ttl:           10800,
			expectDeleted: true,
		},
		{
			name:          "skips valid claim",
			claims:        []client.Object{newTestClaim("valid", now.Add(-30*time.Minute))},
			ttl:           10800,
			expectSkipped: true,
		},
		{
			name: "no claim exists",
			ttl:  10800,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			scheme.AddKnownTypeWithName(fakeGVK, &FakeNoSQLClaim{})

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.claims...).Build()

			// Reset metrics between runs
			DeletedClaims.Reset()
			SkippedClaims.Reset()
			ReconcileDuration = prometheus.NewHistogram(
				prometheus.HistogramOpts{
					Name:    "nosqlclaim_reconcile_duration_seconds",
					Help:    "Duration of NoSQLClaim reconciliation",
					Buckets: prometheus.DefBuckets,
				},
			)

			logger := zap.New(zap.UseDevMode(true))
			r := &NoSQLClaimReconciler{
				Client:     c,
				Log:        logger,
				TTLSeconds: tt.ttl,
			}

			var req reconcile.Request
			if len(tt.claims) > 0 {
				req = reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      tt.claims[0].GetName(),
						Namespace: tt.claims[0].GetNamespace(),
					},
				}
			} else {
				req = reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "missing",
						Namespace: "default",
					},
				}
			}

			_, err := r.Reconcile(context.Background(), req)
			require.NoError(t, err)

			if tt.expectDeleted {
				require.Equal(t, float64(1), testutil.ToFloat64(DeletedClaims.WithLabelValues()))
			} else {
				require.Equal(t, float64(0), testutil.ToFloat64(DeletedClaims.WithLabelValues()))
			}

			if tt.expectSkipped {
				require.Equal(t, float64(1), testutil.ToFloat64(SkippedClaims.WithLabelValues()))
			} else {
				require.Equal(t, float64(0), testutil.ToFloat64(SkippedClaims.WithLabelValues()))
			}

			require.Equal(t, 1, testutil.CollectAndCount(ReconcileDuration))
		})
	}
}
