variable "project" { type = string }

resource "aws_ecr_repository" "api" {
  name                 = "${var.project}-api"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = { Project = var.project }
}

resource "aws_ecr_repository" "mlflow" {
  name                 = "${var.project}-mlflow"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = { Project = var.project }
}

# Lifecycle policy: keep the 10 most recent images per repo to control costs.
resource "aws_ecr_lifecycle_policy" "api" {
  repository = aws_ecr_repository.api.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep last 10 images"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 10
      }
      action = { type = "expire" }
    }]
  })
}

resource "aws_ecr_lifecycle_policy" "mlflow" {
  repository = aws_ecr_repository.mlflow.name
  policy     = aws_ecr_lifecycle_policy.api.policy
}

output "api_repo_url" { value = aws_ecr_repository.api.repository_url }
output "mlflow_repo_url" { value = aws_ecr_repository.mlflow.repository_url }
output "api_repo_arn" { value = aws_ecr_repository.api.arn }
output "mlflow_repo_arn" { value = aws_ecr_repository.mlflow.arn }
