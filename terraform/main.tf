terraform {
  required_providers {
    docker = {
      source  = "kreuzwerker/docker"
      version = "3.0.2"
    }
  }
}

provider "aws" {
  region = var.region
}

data "aws_availability_zones" "available" {
  # Exclude local zones
  filter {
    name   = "opt-in-status"
    values = ["opt-in-not-required"]
  }
}

locals {
  cluster_version = "1.32"
  vpc_cidr = "10.0.0.0/16"
  azs      = slice(data.aws_availability_zones.available.names, 0, 3)
  ecr_claim_controller = "claim-controller"
  ecr_api_server = "api-server"
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "20.35.0"

  cluster_name                   = var.cluster_name
  cluster_version                = local.cluster_version
  cluster_endpoint_public_access = true
  enable_cluster_creator_admin_permissions = false

  cluster_compute_config = {
    enabled    = true
    node_pools = ["general-purpose"]
  }
  
  access_entries = {
    root = {
      principal_arn = "arn:aws:iam::${var.aws_account_id}:root"
      #kubernetes_groups = ["system:masters"]
      policy_associations = {
        admin_policy = {
          policy_arn = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"
          access_scope = {
            type       = "cluster"
          }
        }
      }
    }
    admin = {
      principal_arn = "arn:aws:iam::${var.aws_account_id}:user/${var.aws_iam_user}"
      policy_associations = {
        admin_policy = {
          policy_arn = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"
          access_scope = {
            type       = "cluster"
          }
        }
      }
    }
  }
  
  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets
}

################################################################################
# Supporting Resources
################################################################################

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.0"

  name = var.cluster_name
  cidr = local.vpc_cidr

  azs             = local.azs
  private_subnets = [for k, v in local.azs : cidrsubnet(local.vpc_cidr, 4, k)]
  public_subnets  = [for k, v in local.azs : cidrsubnet(local.vpc_cidr, 8, k + 48)]
  intra_subnets   = [for k, v in local.azs : cidrsubnet(local.vpc_cidr, 8, k + 52)]

  enable_nat_gateway = true
  single_nat_gateway = true

  public_subnet_tags = {
    "kubernetes.io/role/elb" = 1
  }

  private_subnet_tags = {
    "kubernetes.io/role/internal-elb" = 1
  }
}

resource "aws_iam_role" "irsa" {
  assume_role_policy = jsonencode({
    Statement = [{
      Action = "sts:AssumeRoleWithWebIdentity"
      Effect = "Allow"
      Principal = {
        Federated = module.eks.oidc_provider_arn
      }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ecr_pull" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
  role       = aws_iam_role.irsa.name
}

resource "aws_ecr_repository" "claim-controller" {
  name = local.ecr_claim_controller
}

resource "aws_ecr_repository" "api-server" {
  name = local.ecr_api_server
}

data "aws_caller_identity" "current" {}
data "aws_ecr_authorization_token" "token" {}

provider "docker" {
  registry_auth {
    address  = "${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.region}.amazonaws.com"
    username = data.aws_ecr_authorization_token.token.user_name
    password = data.aws_ecr_authorization_token.token.password
  }
}

resource "docker_image" "controller" {
  name = "${aws_ecr_repository.claim-controller.repository_url}:latest"
  build {
    context = "${path.module}/../claim-controller"
  }
}

resource "docker_registry_image" "controller_app" {
  name          = docker_image.controller.name
  keep_remotely = false # Prevent deletion on destroy
}

resource "docker_image" "api_server" {
  name = "${aws_ecr_repository.api-server.repository_url}:latest"
  build {
    context = "${path.module}/../api-server"
  }
}

resource "docker_registry_image" "api_server_app" {
  name          = docker_image.api_server.name
  keep_remotely = false # Prevent deletion on destroy
}

provider "helm" {
  kubernetes {
    host                   = module.eks.cluster_endpoint
    cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)
    exec {
      api_version = "client.authentication.k8s.io/v1beta1"
      command     = "aws"
      args        = ["eks", "get-token", "--cluster-name", module.eks.cluster_name]
    }
  }
}

resource "helm_release" "crossplane" {
  name       = "crossplane"
  repository = "https://charts.crossplane.io/stable"
  chart      = "crossplane"
  namespace  = "crossplane-system"
  create_namespace = true
  wait               = true
  timeout            = 120
  atomic             = true
  cleanup_on_fail    = true

  depends_on = [module.eks]
}

resource "null_resource" "kubectl_apply" {
  depends_on = [helm_release.crossplane]

  provisioner "local-exec" {
    command = <<-EOT
      aws eks update-kubeconfig --region ${var.region} --name ${var.cluster_name}

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

resource "helm_release" "claim-controller" {
  depends_on = [
    module.eks, 
    aws_ecr_repository.claim-controller, 
    docker_image.controller, 
    helm_release.crossplane, 
    null_resource.kubectl_apply
  ]

  name       = "claim-controller"
  chart      = "${path.module}/../claim-controller-chart"
  namespace  = "crossplane-system"
  create_namespace = true
  atomic = true
  cleanup_on_fail = true

  values = [
    file("${path.module}/../claim-controller-chart/values.yaml")
  ]

  set {
    name  = "image.repository"
    value = aws_ecr_repository.claim-controller.repository_url
  }

  set {
    name  = "image.pullPolicy"
    value = "Always" # pull from ECR
  }

  set {
    name = "serviceAccount.annotations.eks\\.amazonaws\\.com/role-arn"
    value = aws_iam_role.irsa.arn
  }
}

resource "helm_release" "api-server" {
  depends_on = [
    module.eks, 
    aws_ecr_repository.api-server, 
    docker_image.api_server, 
    helm_release.crossplane, 
    null_resource.kubectl_apply
  ]

  name       = "api-server"
  chart      = "${path.module}/../api-server-chart"
  namespace  = "crossplane-system"
  create_namespace = true
  atomic = true
  cleanup_on_fail = true

  values = [
    file("${path.module}/../api-server-chart/values.yaml")
  ]

  set {
    name  = "image.repository"
    value = aws_ecr_repository.api-server.repository_url
  }

  set {
    name  = "image.pullPolicy"
    value = "Always" # pull from ECR
  }

  set {
    name = "serviceAccount.annotations.eks\\.amazonaws\\.com/role-arn"
    value = aws_iam_role.irsa.arn
  }
}
