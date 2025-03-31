package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func newTestClaim(name string, annotations map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.Object = map[string]any{
		"apiVersion": fmt.Sprintf("%s/%s", APIGroup, APIVersion),
		"kind":       "Storage",
		"metadata": map[string]any{
			"name":        name,
			"namespace":   "default",
			"annotations": annotations,
		},
	}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   APIGroup,
		Version: APIVersion,
		Kind:    "Storage",
	})
	return obj
}

// Wrapper that injects GVK into unstructured objects before fake client Get
type gvkAwareClient struct {
	client.Client
	gvk schema.GroupVersionKind
}

func (c *gvkAwareClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		u.SetGroupVersionKind(c.gvk)
	}
	return c.Client.Get(ctx, key, obj, opts...)
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

	// Shared scheme
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{
		Group:   APIGroup,
		Version: APIVersion,
		Kind:    "Storage",
	}, &unstructured.Unstructured{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert []client.Object to []runtime.Object
			runtimeObjs := make([]runtime.Object, len(tt.claims))
			for i, obj := range tt.claims {
				runtimeObjs[i] = obj
			}

			// Base fake client
			baseClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(runtimeObjs...).
				Build()

			// Wrap with GVK-injecting test client
			testClient := &gvkAwareClient{
				Client: baseClient,
				gvk: schema.GroupVersionKind{
					Group:   APIGroup,
					Version: APIVersion,
					Kind:    "Storage",
				},
			}

			// Reset metrics
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

			// Build reconciler
			r := &ClaimReconciler{
				Client:     testClient,
				Log:        zap.New(zap.UseDevMode(true)),
				TTLSeconds: tt.ttl,
			}

			// Build request
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

			// Reconcile
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
