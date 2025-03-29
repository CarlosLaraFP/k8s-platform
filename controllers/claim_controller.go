package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NoSQLClaimReconciler struct {
	client.Client
	Log        logr.Logger
	TTLSeconds int64
}

func (r *NoSQLClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	defer func() {
		ReconcileDuration.Observe(time.Since(start).Seconds())
	}()

	log := r.Log.WithValues("nosqlclaim", req.NamespacedName)

	claim := &unstructured.Unstructured{}
	claim.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "platform.example.org",
		Version: "v1alpha1",
		Kind:    "NoSQLClaim",
	})

	err := r.Get(ctx, req.NamespacedName, claim)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("NoSQLClaim not found, skipping")
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get NoSQLClaim")
		return ctrl.Result{}, err
	}

	created := claim.GetCreationTimestamp()
	age := time.Since(created.Time)

	log.Info("reconciled NoSQLClaim", "age", age.String())

	if age.Seconds() >= float64(r.TTLSeconds) {
		log.Info("deleting expired NoSQLClaim", "age", age.String())
		DeletedClaims.Inc()

		if err := r.Delete(ctx, claim); err != nil {
			log.Error(err, "failed to delete expired NoSQLClaim")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	SkippedClaims.Inc()

	remaining := time.Duration(r.TTLSeconds)*time.Second - age
	log.Info("requeueing after", "remaining", remaining)

	return ctrl.Result{RequeueAfter: remaining}, nil
}

func (r *NoSQLClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "platform.example.org/v1alpha1",
				"kind":       "NoSQLClaim",
			},
		}).
		Complete(r)
}
