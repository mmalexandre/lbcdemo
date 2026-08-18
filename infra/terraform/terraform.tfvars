# Copy this file to terraform.tfvars and fill in your values.
# NEVER commit terraform.tfvars — it is listed in .gitignore.

aws_region         = "us-east-1"
project            = "lbcdemo"
node_instance_type = "t3.medium"
db_password = "pg5gx8dboivTtwTPqGoE"
alb_dns_name = "k8s-lbcdemo-9f83a201bd-609550011.us-east-1.elb.amazonaws.com"
