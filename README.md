# ⚙️ Crossplane AWS Resource Lifecycle Controller

This project showcases a **production-grade platform engineering controller** built on Kubernetes and Crossplane. It automates the lifecycle of **ephemeral AWS infrastructure**, such as S3 buckets and DynamoDB tables, based on TTL (time-to-live) logic defined at the claim level.

This is part of a larger project: Building a cloud-agnostic developer self-service platform with Crossplane, ArgoCD, AWS, and Terraform.

---

## 🚀 Features

- **Crossplane-native AWS resource provisioning** via `Composition` and `XRD` definitions  
- **Custom Kubebuilder controller** to automatically delete transient claims after `T` hours  
- **Prometheus metrics** exported for reconciliation counts, durations, and cleanup results  
- **Helm-packaged** for seamless deployment into any cluster  
- **Terraform-powered EKS cluster provisioning** with Helm installing:
  - Crossplane
  - ArgoCD (for GitOps)
  - This controller
- **Makefile-driven development & GitHub Actions CI**
- **Tested on local KinD** and production-grade EKS environments
- **Unit tests using `controller-runtime` fake client** and Prometheus test harness

---

## Setup

```bash
make deploy # after cloning the repo, replace mock-aws-credentials.txt
make apply
make destroy
