variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-west-2"
}

variable "cluster_name" {
  description = "Name of the EKS cluster"
  type        = string
  default     = "k8s-platform"
}

variable "aws_account_id" {
    description = "AWS account ID (deployment target)"
    type = string
}

variable "aws_iam_user" {
    description = "Name of the IAM User (after 'aws configure' it can be obtained from aws sts get-caller-identity)"
    type = string
}
