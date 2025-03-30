package controllers

import (
	"context"
	"fmt"
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

// Minimal fake struct that mimics our Claim CRD
type FakeClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              map[string]string `json:"spec,omitempty"`
}

func (f *FakeClaim) DeepCopyObject() runtime.Object {
	copy := *f
	return &copy
}

var fakeGVK = schema.GroupVersionKind{
	Group:   APIGroup,
	Version: APIVersion,
	Kind:    ClaimName,
}

func newTestClaim(name string, annotations map[string]string) *FakeClaim {
	return &FakeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       ClaimName,
			APIVersion: fmt.Sprintf("%s/%s", APIGroup, APIVersion),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "default",
			Annotations: annotations,
		},
	}
}

func TestClaimReconciler_Reconcile(t *testing.T) {
	now := time.Now().Local().Format(time.RFC3339)
	expired := time.Now().Local().Add(-1 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		name          string
		claims        []client.Object
		ttl           int64
		expectUpdated bool
		expectDeleted bool
		expectSkipped bool
	}{
		{
			name:          "deletes expired claim",
			claims:        []client.Object{newTestClaim("expired", map[string]string{CreationAnnotation: expired})},
			ttl:           TTLSeconds,
			expectDeleted: true,
		},
		{
			name:          "skips valid claim",
			claims:        []client.Object{newTestClaim("valid", map[string]string{CreationAnnotation: now})},
			ttl:           TTLSeconds,
			expectSkipped: true,
		},
		{
			name:          "updates new claim",
			claims:        []client.Object{newTestClaim("new", nil)},
			ttl:           TTLSeconds,
			expectUpdated: true,
		},
		{
			name: "no claim exists",
			ttl:  TTLSeconds,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			scheme.AddKnownTypeWithName(fakeGVK, &FakeClaim{})

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.claims...).Build()

			// Reset metrics between runs
			UpdatedClaims.Reset()
			DeletedClaims.Reset()
			SkippedClaims.Reset()
			ReconcileDuration = prometheus.NewHistogram(
				prometheus.HistogramOpts{
					Name:    "claim_reconcile_duration_seconds",
					Help:    "Duration of Claim reconciliation",
					Buckets: prometheus.DefBuckets,
				},
			)

			logger := zap.New(zap.UseDevMode(true))
			r := &StorageReconciler{
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

			if tt.expectUpdated {
				require.Equal(t, float64(1), testutil.ToFloat64(UpdatedClaims.WithLabelValues()))
			} else {
				require.Equal(t, float64(0), testutil.ToFloat64(UpdatedClaims.WithLabelValues()))
			}

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
