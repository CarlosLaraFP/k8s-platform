output "kubeconfig_command" {
  value = "aws eks update-kubeconfig --region ${var.region} --name ${var.cluster_name}"
}
output "cluster_name"     { value = module.eks.cluster_name }
output "cluster_endpoint" { value = module.eks.cluster_endpoint }
output "cluster_ca"       { value = module.eks.cluster_certificate_authority_data }
output "vpc_id"           { value = module.vpc.vpc_id }
output "region"           { value = var.region }
output "irsa_role_arn"    { value = aws_iam_role.irsa.arn }
output "api_server_image" { value = docker_image.api_server.name }
output "claim_controller_image" { value = docker_image.claim_controller.name }
output "aws_lb_controller_irsa_arn" { value = aws_iam_role.aws_lb_controller_irsa.arn }
