variable "project"    { type = string }
variable "aws_region" { type = string }

# Placeholder secrets — the actual values are stored/updated manually or via
# the deploy pipeline after provisioning.  Terraform creates the secret
# containers so that the ESO ClusterSecretStore can resolve them.
locals {
  secrets = {
    "openai-api-key"        = "REPLACE_WITH_OPENAI_API_KEY"
    "session-secret"        = "REPLACE_WITH_SESSION_SECRET_MIN_32_CHARS"
    "database-url"          = "REPLACE_WITH_DATABASE_URL"
    "mlflow-tracking-token" = "REPLACE_WITH_MLFLOW_TOKEN_OR_EMPTY"
  }
}

resource "aws_secretsmanager_secret" "this" {
  for_each                = local.secrets
  name                    = "${var.project}/${each.key}"
  recovery_window_in_days = 0   # Allow immediate deletion during development
  tags                    = { Project = var.project }
}

resource "aws_secretsmanager_secret_version" "this" {
  for_each      = local.secrets
  secret_id     = aws_secretsmanager_secret.this[each.key].id
  secret_string = jsonencode({ value = each.value })

  lifecycle {
    # Prevent Terraform from overwriting secrets that have been updated outside
    # of Terraform (e.g. via AWS console or CI pipeline).
    ignore_changes = [secret_string]
  }
}

output "secret_arns" {
  value = { for k, v in aws_secretsmanager_secret.this : k => v.arn }
}
