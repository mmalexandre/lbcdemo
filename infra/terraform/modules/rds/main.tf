variable "project"         { type = string }
variable "vpc_id"          { type = string }
variable "subnet_ids"      { type = list(string) }
variable "eks_node_sg_id"  { type = string }
variable "db_password" {
  type      = string
  sensitive = true
}

# ── Subnet group ──────────────────────────────────────────────────────────────
resource "aws_db_subnet_group" "this" {
  name       = "${var.project}-rds"
  subnet_ids = var.subnet_ids
  tags       = { Project = var.project }
}

# ── Security group: allow EKS nodes on port 5432 only ────────────────────────
resource "aws_security_group" "rds" {
  name   = "${var.project}-rds"
  vpc_id = var.vpc_id

  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [var.eks_node_sg_id]
    description     = "Allow EKS nodes to reach PostgreSQL"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Project = var.project }
}

# ── RDS PostgreSQL instance ───────────────────────────────────────────────────
resource "aws_db_instance" "this" {
  identifier              = "${var.project}-postgres"
  engine                  = "postgres"
  engine_version          = "16"
  instance_class          = "db.t3.micro"
  allocated_storage       = 20
  storage_type            = "gp2"
  storage_encrypted       = true

  db_name  = "appdb"
  username = "postgres"
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  # Create the mlflow database automatically via init scripts is not supported
  # by RDS.  The mlflow DB is created by a post-provision script (see DEPLOY.md).
  publicly_accessible    = false
  skip_final_snapshot    = true   # Set to false for production

  tags = { Project = var.project }
}

output "endpoint" {
  value     = aws_db_instance.this.endpoint
  sensitive = true
}
output "db_name" { value = aws_db_instance.this.db_name }
