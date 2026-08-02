variable "project"            { type = string }
variable "aws_region"         { type = string }
variable "vpc_id"             { type = string }
variable "subnet_ids"         { type = list(string) }
variable "node_instance_type" { type = string }

data "aws_eks_cluster_auth" "this" {
  name = aws_eks_cluster.this.name
}

# ── IAM role for the EKS control plane ───────────────────────────────────────
resource "aws_iam_role" "cluster" {
  name = "${var.project}-eks-cluster"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "eks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "cluster_policy" {
  role       = aws_iam_role.cluster.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
}

# ── EKS cluster ───────────────────────────────────────────────────────────────
resource "aws_eks_cluster" "this" {
  name     = "${var.project}-cluster"
  role_arn = aws_iam_role.cluster.arn
  version  = "1.31"

  vpc_config {
    subnet_ids              = var.subnet_ids
    endpoint_public_access  = true
    endpoint_private_access = true
  }

  depends_on = [aws_iam_role_policy_attachment.cluster_policy]

  tags = { Project = var.project }
}

# ── OIDC provider (required for IRSA) ────────────────────────────────────────
data "tls_certificate" "eks" {
  url = aws_eks_cluster.this.identity[0].oidc[0].issuer
}

resource "aws_iam_openid_connect_provider" "eks" {
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.eks.certificates[0].sha1_fingerprint]
  url             = aws_eks_cluster.this.identity[0].oidc[0].issuer
}

# ── IAM role for worker nodes ─────────────────────────────────────────────────
resource "aws_iam_role" "node" {
  name = "${var.project}-eks-node"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "node_worker" {
  role       = aws_iam_role.node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
}
resource "aws_iam_role_policy_attachment" "node_cni" {
  role       = aws_iam_role.node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
}
resource "aws_iam_role_policy_attachment" "node_ecr" {
  role       = aws_iam_role.node.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

# ── Managed node group ────────────────────────────────────────────────────────
resource "aws_eks_node_group" "default" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "${var.project}-nodes"
  node_role_arn   = aws_iam_role.node.arn
  subnet_ids      = var.subnet_ids
  instance_types  = [var.node_instance_type]

  scaling_config {
    desired_size = 2
    min_size     = 2
    max_size     = 5
  }

  update_config {
    max_unavailable = 1
  }

  depends_on = [
    aws_iam_role_policy_attachment.node_worker,
    aws_iam_role_policy_attachment.node_cni,
    aws_iam_role_policy_attachment.node_ecr,
  ]

  tags = { Project = var.project }
}

# ── Security group for nodes (referenced by RDS module) ───────────────────────
data "aws_security_group" "nodes" {
  filter {
    name   = "tag:eks:nodegroup-name"
    values = ["${var.project}-nodes"]
  }
  depends_on = [aws_eks_node_group.default]
}

output "cluster_name"             { value = aws_eks_cluster.this.name }
output "cluster_endpoint"         { value = aws_eks_cluster.this.endpoint }
output "oidc_provider_arn"        { value = aws_iam_openid_connect_provider.eks.arn }
output "oidc_provider_url"        { value = aws_iam_openid_connect_provider.eks.url }
output "node_security_group_id"   { value = data.aws_security_group.nodes.id }
