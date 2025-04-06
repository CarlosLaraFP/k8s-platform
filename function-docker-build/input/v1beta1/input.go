// Package v1beta1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=template.fn.crossplane.io
// +versionName=v1beta1
package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// ModelDeployment XR is the input to this Function.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type ModelDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ModelDeploymentSpec `json:"spec"`
}

type ModelDeploymentSpec struct {
	UserName         string `json:"userName"`
	RequirementsPath string `json:"requirementsPath"`
}

// When you edit files under the input directory you must update some generated files by running go generate. See input/generate.go for details.
// go generate ./...
