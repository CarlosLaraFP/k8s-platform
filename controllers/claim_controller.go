package controllers

import (
	"context"
	"fmt"
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
	APIGroup           = "platform.example.org"
	APIVersion         = "v1alpha1"
	ClaimName          = "Storage"
	TTLSeconds         = 600 // 10 minutes
	CreationAnnotation = "platform.example.org/creationTimestamp"
)

type StorageReconciler struct {
	client.Client
	Log        logr.Logger
	TTLSeconds int64
}

func (r *StorageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	/*
		retry.RetryOnConflict is only needed when we are performing update or patch operations on Kubernetes objects —
		especially if those operations use .Update() or .Patch() on live objects that might change concurrently.
	*/
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

	creationTimeStr, exists := claim.GetAnnotations()[CreationAnnotation] // will not panic even if annotations is nil

	if !exists {
		now := time.Now().Local().Format(time.RFC3339)
		annotations := claim.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[CreationAnnotation] = now
		claim.SetAnnotations(annotations)

		if err := r.Update(ctx, claim); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add creation annotation: %w", err)
		}

		UpdatedClaims.WithLabelValues().Inc()
		// Update will trigger a new reconcile
		return ctrl.Result{}, nil
	}

	creationTime, err := time.Parse(time.RFC3339, creationTimeStr)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid creation timestamp: %w", err)
	}

	log.Info("reconciled", "claim", ClaimName, "age", creationTimeStr)

	age := time.Since(creationTime)

	// Check if claim is older than max age
	if age.Seconds() >= float64(r.TTLSeconds) {
		log.Info("deleting expired", "claim", ClaimName, "age", age.String())

		// Delete is idempotent
		if err := r.Delete(ctx, claim); err != nil {
			log.Error(err, "failed to delete expired Claim", "claim", ClaimName, "name", req.NamespacedName)
			return ctrl.Result{}, err
		}

		DeletedClaims.WithLabelValues().Inc()

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
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
