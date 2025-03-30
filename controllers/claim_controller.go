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
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

const (
	APIGroup   = "platform.example.org"
	APIVersion = "v1alpha1"
	ClaimName  = "Storage"
	TTLSeconds = 3600
)

type StorageReconciler struct {
	client.Client
	Log        logr.Logger
	TTLSeconds int64
}

func (r *StorageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	defer func() {
		ReconcileDuration.Observe(time.Since(start).Seconds())
	}()

	log := r.Log.WithValues(ClaimName, req.NamespacedName)

	claim := &unstructured.Unstructured{}
	claim.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   APIGroup,
		Version: APIVersion,
		Kind:    ClaimName,
	})

	err := r.Get(ctx, req.NamespacedName, claim)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Claim not found, skipping", "claim", ClaimName, "name", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get Claim", "claim", ClaimName, "name", req.NamespacedName)
		return ctrl.Result{}, err
	}

	created := claim.GetCreationTimestamp()
	age := time.Since(created.Time)

	log.Info("reconciled", "claim", ClaimName, "age", age.String())

	if age.Seconds() >= float64(r.TTLSeconds) {
		log.Info("deleting expired", "claim", ClaimName, "age", age.String())

		DeletedClaims.WithLabelValues().Inc()

		if err := r.Delete(ctx, claim); err != nil {
			log.Error(err, "failed to delete expired Claim", "claim", ClaimName, "name", req.NamespacedName)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	SkippedClaims.WithLabelValues().Inc()

	remaining := time.Duration(r.TTLSeconds)*time.Second - age
	log.Info("requeueing after", "remaining", remaining)

	return ctrl.Result{RequeueAfter: remaining}, nil
}

func (r *StorageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	claim := &unstructured.Unstructured{}
	claim.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   APIGroup,
		Version: APIVersion,
		Kind:    ClaimName,
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(claim).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Complete(r)
}
