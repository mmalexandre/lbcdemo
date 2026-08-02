variable "project"               { type = string }
variable "aws_region"            { type = string }
variable "aws_account_id"        { type = string }
variable "eks_oidc_provider_arn" { type = string }
variable "eks_oidc_provider_url" { type = string }
variable "ecr_api_repo_arn"      { type = string }
variable "ecr_mlflow_repo_arn"   { type = string }
variable "frontend_bucket_arn"   { type = string }
variable "mlflow_bucket_arn"     { type = string }
variable "artifacts_bucket_name" { type = string }

locals {
  oidc_host = replace(var.eks_oidc_provider_url, "https://", "")
}

# ── 1. GitHub Actions OIDC role ───────────────────────────────────────────────
# Allows GitHub Actions to push to ECR, deploy to EKS, and sync to S3 without
# storing long-lived AWS credentials as GitHub secrets.
data "aws_iam_policy_document" "github_assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = ["arn:aws:iam::${var.aws_account_id}:oidc-provider/token.actions.githubusercontent.com"]
    }
    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      # Restrict to your repo — update the value in terraform.tfvars
      values = ["repo:*:ref:refs/heads/main"]
    }
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "github_actions" {
  name               = "${var.project}-github-actions"
  assume_role_policy = data.aws_iam_policy_document.github_assume.json
}

data "aws_iam_policy_document" "github_actions" {
  # ECR: authenticate and push
  statement {
    effect    = "Allow"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }
  statement {
    effect = "Allow"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:CompleteLayerUpload",
      "ecr:InitiateLayerUpload",
      "ecr:PutImage",
      "ecr:UploadLayerPart",
      "ecr:BatchGetImage",
      "ecr:GetDownloadUrlForLayer",
    ]
    resources = [var.ecr_api_repo_arn, var.ecr_mlflow_repo_arn]
  }
  # EKS: describe cluster for kubeconfig
  statement {
    effect    = "Allow"
    actions   = ["eks:DescribeCluster"]
    resources = ["arn:aws:eks:${var.aws_region}:${var.aws_account_id}:cluster/${var.project}-cluster"]
  }
  # S3: sync frontend build to the static hosting bucket
  statement {
    effect = "Allow"
    actions = [
      "s3:PutObject",
      "s3:DeleteObject",
      "s3:ListBucket",
    ]
    resources = [var.frontend_bucket_arn, "${var.frontend_bucket_arn}/*"]
  }
  # CloudFront: invalidate cache after frontend deploy
  statement {
    effect    = "Allow"
    actions   = ["cloudfront:CreateInvalidation"]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "github_actions" {
  name   = "permissions"
  role   = aws_iam_role.github_actions.id
  policy = data.aws_iam_policy_document.github_actions.json
}

# ── 2. External Secrets Operator / API pod role (IRSA) ───────────────────────
data "aws_iam_policy_document" "eso_assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [var.eks_oidc_provider_arn]
    }
    condition {
      test     = "StringEquals"
      variable = "${local.oidc_host}:sub"
      values   = ["system:serviceaccount:lbcdemo:api"]
    }
    condition {
      test     = "StringEquals"
      variable = "${local.oidc_host}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "eso" {
  name               = "${var.project}-eso"
  assume_role_policy = data.aws_iam_policy_document.eso_assume.json
}

data "aws_iam_policy_document" "eso" {
  statement {
    effect = "Allow"
    actions = [
      "secretsmanager:GetSecretValue",
      "secretsmanager:DescribeSecret",
    ]
    resources = ["arn:aws:secretsmanager:${var.aws_region}:${var.aws_account_id}:secret:${var.project}/*"]
  }
}

resource "aws_iam_role_policy" "eso" {
  name   = "secrets-manager-read"
  role   = aws_iam_role.eso.id
  policy = data.aws_iam_policy_document.eso.json
}

# ── 3. MLflow pod S3 role (IRSA) ──────────────────────────────────────────────
data "aws_iam_policy_document" "mlflow_assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [var.eks_oidc_provider_arn]
    }
    condition {
      test     = "StringEquals"
      variable = "${local.oidc_host}:sub"
      values   = ["system:serviceaccount:lbcdemo:mlflow"]
    }
    condition {
      test     = "StringEquals"
      variable = "${local.oidc_host}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "mlflow_s3" {
  name               = "${var.project}-mlflow-s3"
  assume_role_policy = data.aws_iam_policy_document.mlflow_assume.json
}

data "aws_iam_policy_document" "mlflow_s3" {
  statement {
    effect = "Allow"
    actions = [
      "s3:GetObject",
      "s3:PutObject",
      "s3:DeleteObject",
      "s3:ListBucket",
    ]
    resources = [var.mlflow_bucket_arn, "${var.mlflow_bucket_arn}/*"]
  }
}

resource "aws_iam_role_policy" "mlflow_s3" {
  name   = "s3-artifacts"
  role   = aws_iam_role.mlflow_s3.id
  policy = data.aws_iam_policy_document.mlflow_s3.json
}

output "github_actions_role_arn" { value = aws_iam_role.github_actions.arn }
output "eso_role_arn"            { value = aws_iam_role.eso.arn }
output "mlflow_s3_role_arn"      { value = aws_iam_role.mlflow_s3.arn }
