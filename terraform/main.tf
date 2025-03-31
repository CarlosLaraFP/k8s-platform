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
  ecr_repo = "claim-controller"
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
    /*
    dev-user = {
      principal_arn = "arn:aws:iam::${var.aws_account_id}:user/${var.aws_iam_user}"
      policy_associations = {
        admin_policy = {
          policy_arn = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"
          username = "${var.aws_iam_user}"
          access_scope = {
            namespaces = ["*"]
            type       = "namespace"
          }
        }
      }
    }
    */
  }
  
  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets
}

#module "disabled_eks" {
#  source = "terraform-aws-modules/eks/aws"
#
#  create = false
#}

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

resource "aws_ecr_repository" "claim-controller" {
  name = local.ecr_repo
}

resource "helm_release" "custom_chart" {
  depends_on = [module.eks, aws_ecr_repository.crossplane]

  name       = "claim-controller"
  chart      = "${path.module}/../chart"
  namespace  = "crossplane-system"
  create_namespace = true

  values = [
    file("${path.module}/../chart/values.yaml")
  ]

  set {
    name  = "image.repository"
    value = aws_ecr_repository.claim-controller.repository_url
  }

  set {
    name  = "image.pullPolicy"
    value = "Always" # pull from ECR
  }
}
