variable "project" { type = string }

# ── Frontend bucket (private – served only via CloudFront OAC) ────────────────
resource "aws_s3_bucket" "frontend" {
  bucket        = "${var.project}-frontend"
  force_destroy = true
  tags          = { Project = var.project }
}

resource "aws_s3_bucket_public_access_block" "frontend" {
  bucket                  = aws_s3_bucket.frontend.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_versioning" "frontend" {
  bucket = aws_s3_bucket.frontend.id
  versioning_configuration { status = "Enabled" }
}

# ── MLflow artefacts bucket ───────────────────────────────────────────────────
resource "aws_s3_bucket" "mlflow" {
  bucket        = "${var.project}-mlflow-artifacts"
  force_destroy = true
  tags          = { Project = var.project }
}

resource "aws_s3_bucket_public_access_block" "mlflow" {
  bucket                  = aws_s3_bucket.mlflow.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

output "frontend_bucket_id"              { value = aws_s3_bucket.frontend.id }
output "frontend_bucket_arn"             { value = aws_s3_bucket.frontend.arn }
output "frontend_bucket_regional_domain" { value = aws_s3_bucket.frontend.bucket_regional_domain_name }
output "mlflow_bucket_id"                { value = aws_s3_bucket.mlflow.id }
output "mlflow_bucket_arn"               { value = aws_s3_bucket.mlflow.arn }
