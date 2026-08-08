variable "aws_region" {
  description = "AWS region to deploy all resources into."
  type        = string
  default     = "us-east-1"
}

variable "project" {
  description = "Short project name used as a prefix for all resource names."
  type        = string
  default     = "lbcdemo"
}

variable "node_instance_type" {
  description = "EC2 instance type for EKS worker nodes."
  type        = string
  default     = "t3.small"
}

variable "db_password" {
  description = "Master password for the RDS PostgreSQL instance.  Provide via TF_VAR_db_password or terraform.tfvars (do NOT commit)."
  type        = string
  sensitive   = true
}

variable "alb_dns_name" {
  description = "DNS name of the ALB created by AWS Load Balancer Controller for the API ingress."
  type        = string
  default     = ""
}
