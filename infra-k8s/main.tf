terraform {
  required_providers {
    helm = {
      source  = "hashicorp/helm"
      version = "2.13.2"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "2.27.0"
    }
  }
}

# Reference local state from infra-eks
data "terraform_remote_state" "eks" {
  backend = "local"
  config = {
    path = "${path.module}/../infra-eks/terraform.tfstate"
  }
}

provider "kubernetes" {
  host                   = data.terraform_remote_state.eks.outputs.cluster_endpoint
  cluster_ca_certificate = base64decode(data.terraform_remote_state.eks.outputs.cluster_ca)
  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "aws"
    args        = ["eks", "get-token", "--cluster-name", data.terraform_remote_state.eks.outputs.cluster_name]
  }
}

provider "helm" {
  kubernetes {
    host                   = data.terraform_remote_state.eks.outputs.cluster_endpoint
    cluster_ca_certificate = base64decode(data.terraform_remote_state.eks.outputs.cluster_ca)
    exec {
      api_version = "client.authentication.k8s.io/v1beta1"
      command     = "aws"
      args        = ["eks", "get-token", "--cluster-name", data.terraform_remote_state.eks.outputs.cluster_name]
    }
  }
}

resource "helm_release" "crossplane" {
  name       = "crossplane"
  repository = "https://charts.crossplane.io/stable"
  chart      = "crossplane"
  namespace  = "crossplane-system"
  create_namespace = true
  atomic = true
  cleanup_on_fail = true
  depends_on = [ null_resource.lb_controller_wait ]
}

resource "null_resource" "kubectl_apply" {
  depends_on = [helm_release.crossplane]

  provisioner "local-exec" {
    command = <<-EOT
      kubectl wait --for=condition=Available deployment/crossplane -n crossplane-system --timeout=120s
	    kubectl apply -f ${path.module}/../infra/s3-provider.yaml 
	    kubectl apply -f ${path.module}/../infra/dynamodb-provider.yaml
      kubectl apply -f ${path.module}/../infra/ec2-provider.yaml
	    kubectl wait --for=condition=Healthy provider/provider-aws-dynamodb --timeout=180s
      kubectl wait --for=condition=Installed provider/provider-aws-dynamodb --timeout=180s
      kubectl create secret generic aws-secret -n crossplane-system --from-file=creds=${path.module}/../aws-credentials.txt
      kubectl apply -f ${path.module}/../infra/provider-config.yaml
      
      kubectl apply -f ${path.module}/../infra/functions/patch-and-transform.yaml
      kubectl apply -f ${path.module}/../infra/storage-xrd.yaml
      kubectl apply -f ${path.module}/../infra/storage-composition.yaml
      kubectl apply -f ${path.module}/../infra/compute-xrd.yaml
      kubectl apply -f ${path.module}/../infra/compute-composition.yaml
      kubectl apply -f ${path.module}/../infra/modeldeployment-xrd.yaml
      kubectl apply -f ${path.module}/../infra/modeldeployment-composition.yaml

      kubectl create namespace argocd
      kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
      kubectl wait --for=condition=available --timeout=180s deployment/argocd-server -n argocd
      kubectl apply -f ${path.module}/../infra/argocd-project.yaml
      kubectl apply -f ${path.module}/../infra/argocd-app.yaml
    EOT
  }
}

resource "helm_release" "claim_controller" {
  name       = "claim-controller"
  chart      = "${path.module}/../claim-controller-chart"
  namespace  = "crossplane-system"
  create_namespace = true
  atomic = true
  cleanup_on_fail = true

  values = [file("${path.module}/../claim-controller-chart/values.yaml")]

  set {
    name  = "image.uri"
    value = data.terraform_remote_state.eks.outputs.claim_controller_image
  }

  set {
    name = "serviceAccount.annotations.eks\\.amazonaws\\.com/role-arn"
    value = data.terraform_remote_state.eks.outputs.irsa_role_arn
  }

  depends_on = [null_resource.kubectl_apply]
}

resource "helm_release" "api_server" {
  name       = "api-server"
  chart      = "${path.module}/../api-server-chart"
  namespace  = "crossplane-system"
  create_namespace = true
  atomic = true
  cleanup_on_fail = true

  values = [file("${path.module}/../api-server-chart/values.yaml")]

  set {
    name  = "image.uri"
    value = data.terraform_remote_state.eks.outputs.api_server_image
  }

  set {
    name = "serviceAccount.annotations.eks\\.amazonaws\\.com/role-arn"
    value = data.terraform_remote_state.eks.outputs.irsa_role_arn
  }

  depends_on = [null_resource.kubectl_apply]
}
