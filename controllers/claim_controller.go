package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
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
		// Retry loop to avoid conflict errors when updating annotations, especially when a Claim is first created
		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			// Re-fetch the latest version
			latest := &unstructured.Unstructured{}
			latest.SetGroupVersionKind(claim.GroupVersionKind())
			if err := r.Get(ctx, req.NamespacedName, latest); err != nil {
				return err
			}

			annotations := latest.GetAnnotations()
			if annotations == nil {
				annotations = map[string]string{}
			}
			annotations[CreationAnnotation] = time.Now().Local().Format(time.RFC3339)
			latest.SetAnnotations(annotations)
			// the Update will trigger a new event, which will be skipped and requeue will be scheduled
			return r.Update(ctx, latest)
		})

		if err != nil {
			log.Error(err, "failed to add creation annotation with retry")
			return ctrl.Result{}, fmt.Errorf("failed to add creation annotation: %w", err)
		}

		UpdatedClaims.WithLabelValues().Inc()
		// Requeue immediately — next loop will add the creation timestamp
		return ctrl.Result{Requeue: true}, nil
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
