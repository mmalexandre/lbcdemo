terraform {
  required_version = ">= 1.6"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.0"
    }
  }

  # Uncomment and configure after creating the state bucket
  # backend "s3" {
  #   bucket         = "lbcdemo-tf-state"
  #   key            = "infra/terraform.tfstate"
  #   region         = "us-east-1"
  #   dynamodb_table = "lbcdemo-tf-locks"
  #   encrypt        = true
  # }
}

provider "aws" {
  region = var.aws_region
}

# ── Networking ────────────────────────────────────────────────────────────────
# Use the default VPC for demo simplicity.  For production replace with a
# dedicated VPC (private subnets for RDS/nodes, public for ALB).
data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

# ── Modules ───────────────────────────────────────────────────────────────────
module "ecr" {
  source      = "./modules/ecr"
  project     = var.project
}

module "eks" {
  source          = "./modules/eks"
  project         = var.project
  aws_region      = var.aws_region
  vpc_id          = data.aws_vpc.default.id
  subnet_ids      = data.aws_subnets.default.ids
  node_instance_type = var.node_instance_type
}

module "rds" {
  source        = "./modules/rds"
  project       = var.project
  vpc_id        = data.aws_vpc.default.id
  subnet_ids    = data.aws_subnets.default.ids
  eks_node_sg_id = module.eks.node_security_group_id
  db_password   = var.db_password
}

module "s3" {
  source  = "./modules/s3"
  project = var.project
}

module "cloudfront" {
  source              = "./modules/cloudfront"
  project             = var.project
  frontend_bucket_id  = module.s3.frontend_bucket_id
  frontend_bucket_arn = module.s3.frontend_bucket_arn
  frontend_bucket_regional_domain = module.s3.frontend_bucket_regional_domain
}

module "iam" {
  source                = "./modules/iam"
  project               = var.project
  aws_region            = var.aws_region
  aws_account_id        = data.aws_caller_identity.current.account_id
  eks_oidc_provider_arn = module.eks.oidc_provider_arn
  eks_oidc_provider_url = module.eks.oidc_provider_url
  ecr_api_repo_arn      = module.ecr.api_repo_arn
  ecr_mlflow_repo_arn   = module.ecr.mlflow_repo_arn
  frontend_bucket_arn   = module.s3.frontend_bucket_arn
  mlflow_bucket_arn     = module.s3.mlflow_bucket_arn
  artifacts_bucket_name = module.s3.mlflow_bucket_id
}

module "secrets" {
  source     = "./modules/secrets"
  project    = var.project
  aws_region = var.aws_region
}

data "aws_caller_identity" "current" {}
