output "cluster_name" {
  value = module.eks.cluster_name
}

output "kubeconfig_command" {
  value = "aws eks update-kubeconfig --region ${var.region} --name ${var.cluster_name}"
}

output "ecr_repo_url" {
    value = aws_ecr_repository.claim-controller.repository_url
}