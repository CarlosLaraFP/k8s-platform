# Cloud-Agnostic Developer Self-Service Kubernetes Platform

Building a developer self-service platform with Crossplane, ArgoCD, AWS, Terraform.

## Features

- GitOps installation of Crossplane via ArgoCD
- Crossplane Composition to provision AWS S3 Buckets
- Developer-facing claim (Bucket CRD)
- Sync via ArgoCD

## Setup

```bash
make apply       # Apply XRD, Composition, Claim, Provider
make argo-app    # Deploy ArgoCD Application
make clean       # Delete resources
