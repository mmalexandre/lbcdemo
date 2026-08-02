output "ecr_api_url" {
  description = "ECR repository URL for the Go API image."
  value       = module.ecr.api_repo_url
}

output "ecr_mlflow_url" {
  description = "ECR repository URL for the custom MLflow image (if needed)."
  value       = module.ecr.mlflow_repo_url
}

output "eks_cluster_name" {
  description = "EKS cluster name — used in aws eks update-kubeconfig."
  value       = module.eks.cluster_name
}

output "eks_cluster_endpoint" {
  description = "EKS API server endpoint."
  value       = module.eks.cluster_endpoint
}

output "rds_endpoint" {
  description = "RDS PostgreSQL endpoint (host:port)."
  value       = module.rds.endpoint
  sensitive   = true
}

output "frontend_bucket" {
  description = "S3 bucket name for the React build artefacts."
  value       = module.s3.frontend_bucket_id
}

output "mlflow_bucket" {
  description = "S3 bucket name for MLflow artefacts."
  value       = module.s3.mlflow_bucket_id
}

output "cloudfront_domain" {
  description = "CloudFront distribution domain — use as the FRONTEND_ORIGIN in the API config."
  value       = module.cloudfront.domain_name
}

output "github_actions_role_arn" {
  description = "IAM role ARN for GitHub Actions OIDC — set as GHA_ROLE_ARN in GitHub Actions variables."
  value       = module.iam.github_actions_role_arn
}

output "eso_role_arn" {
  description = "IAM role ARN for the External Secrets Operator (api service account)."
  value       = module.iam.eso_role_arn
}

output "mlflow_s3_role_arn" {
  description = "IAM role ARN for the MLflow pod's S3 access (IRSA)."
  value       = module.iam.mlflow_s3_role_arn
}
