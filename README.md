# Cloud-Agnostic Developer Self-Service Platform

[![CI](https://github.com/CarlosLaraFP/k8s-platform/actions/workflows/ci.yml/badge.svg)](https://github.com/CarlosLaraFP/k8s-platform/actions)

Built with Terraform, AWS, Kubernetes, Crossplane, and Helm. It includes a Kubebuilder controller that automates the lifecycle of **ephemeral AWS infrastructure**, such as S3 buckets and DynamoDB tables, based on TTL (time-to-live) logic defined at the claim level.

---

## 🚀 Features

- **Terraform** to provision:
    - Karpenter-based EKS Auto cluster
    - Docker image lifecycle management with ECR
    - Crossplane Helm chart for self-service IaC
    - Custom Kubernetes controller Helm chart
    - RBAC & least privilege access
- **Crossplane-native AWS resource provisioning** via `Composition` and `XRD` definitions  
- **Custom Kubebuilder controller** to automatically delete transient claims after `T` hours  
- **Prometheus metrics** exported for reconciliation counts, durations, and cleanup results  
- **Helm-packaged** for seamless deployment into any Kubernetes cluster
- **ArgoCD-driven GitOps** to keep EKS resources up-to-date
- **Makefile-driven development & GitHub Actions CI**
- **Tested on local KinD** and on Terraform-provisioned EKS Auto with real AWS credentials
- **Unit tests using `controller-runtime` fake client** and Prometheus test harness

---

## Setup

```bash
# For local testing with KinD
make deploy # after cloning the repo, replace mock-aws-credentials.txt with aws-credentials.txt
make apply
make destroy

# For cloud testing with EKS
make terraform-apply
make terraform-destroy # cleanup once you are done
