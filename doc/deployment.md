# Deployment Guide

This project deploys a Go API + React frontend to AWS using EKS, ECR, CloudFront, and GitHub Actions.

## Architecture

- **Frontend:** React (Vite) → S3 + CloudFront
- **API:** Go (Gin) → Docker → ECR → EKS (Helm)
- **MLflow:** Docker → ECR → EKS (Helm), artifacts on S3
- **Database:** RDS PostgreSQL (default VPC)
- **Secrets:** AWS Secrets Manager → External Secrets Operator → Kubernetes Secrets
- **CI/CD:** GitHub Actions with OIDC (no stored AWS credentials)
- **Observability:** Prometheus + Grafana (kube-prometheus-stack) + MLflow tracing

## AWS Resources (created by Terraform)

| Resource | Name/Value |
|---|---|
| EKS Cluster | `lbcdemo-cluster` (us-east-1, t3.medium nodes) |
| ECR — API | `587472608760.dkr.ecr.us-east-1.amazonaws.com/lbcdemo-api` |
| ECR — MLflow | `587472608760.dkr.ecr.us-east-1.amazonaws.com/lbcdemo-mlflow` |
| RDS PostgreSQL | `lbcdemo-postgres.cy5s00quiezw.us-east-1.rds.amazonaws.com:5432` |
| S3 — Frontend | `lbcdemo-frontend` |
| S3 — MLflow artifacts | `lbcdemo-mlflow-artifacts` |
| CloudFront | `d13vttbhe09whf.cloudfront.net` |
| GitHub Actions IAM role | `arn:aws:iam::587472608760:role/lbcdemo-github-actions` |
| ESO IAM role | `arn:aws:iam::587472608760:role/lbcdemo-eso` |
| MLflow S3 IAM role | `arn:aws:iam::587472608760:role/lbcdemo-mlflow-s3` |

## One-Time Setup Checklist

### 1. Terraform — provision infrastructure
```bash
cd infra/terraform
terraform init
terraform apply -auto-approve
```
> Requires IAM user with `AdministratorAccess`. Takes ~15–20 min (EKS + RDS are slow).
> After apply, note the outputs — they are used in the steps below.

### 2. Secrets Manager — fill in real values
Go to **AWS Console → Secrets Manager** and update each secret's value:

| Secret name | Value format |
|---|---|
| `lbcdemo/openai-api-key` | `{"value":"sk-..."}` |
| `lbcdemo/session-secret` | `{"value":"<32+ random chars>"}` |
| `lbcdemo/database-url` | `{"value":"postgresql://postgres:<password>@<rds_endpoint>/appdb?sslmode=require"}` |
| `lbcdemo/mlflow-tracking-token` | `{"value":""}` (empty — MLflow is cluster-internal) |

### 3. GitHub Actions — repository variables
Go to **repo Settings → Secrets and variables → Actions → Variables tab** and add:

| Variable | Value |
|---|---|
| `AWS_ACCOUNT_ID` | `587472608760` |
| `AWS_REGION` | `us-east-1` |
| `GHA_ROLE_ARN` | `arn:aws:iam::587472608760:role/lbcdemo-github-actions` |
| `ESO_ROLE_ARN` | `arn:aws:iam::587472608760:role/lbcdemo-eso` |
| `MLFLOW_S3_ROLE_ARN` | `arn:aws:iam::587472608760:role/lbcdemo-mlflow-s3` |
| `EKS_CLUSTER_NAME` | `lbcdemo-cluster` |
| `FRONTEND_BUCKET` | `lbcdemo-frontend` |
| `CLOUDFRONT_DOMAIN` | `d13vttbhe09whf.cloudfront.net` |
| `MLFLOW_ARTIFACTS_BUCKET` | `lbcdemo-mlflow-artifacts` |
| `ECR_API_URL` | `587472608760.dkr.ecr.us-east-1.amazonaws.com/lbcdemo-api` |
| `ECR_MLFLOW_URL` | `587472608760.dkr.ecr.us-east-1.amazonaws.com/lbcdemo-mlflow` |

### 4. RDS — initialize databases
Connect to RDS from within the cluster (or a bastion) and run:
```sql
-- Create MLflow metadata database
CREATE DATABASE mlflow;

-- Run app schema (connects to appdb)
\c appdb
-- then paste contents of db/init.sql
```

### 5. Connect kubectl to EKS
```bash
aws eks update-kubeconfig --region us-east-1 --name lbcdemo-cluster
kubectl get nodes  # should show 2 Ready nodes
```

### 6. Install cluster add-ons (one-time)
```bash
# AWS Load Balancer Controller
helm repo add eks https://aws.github.io/eks-charts && helm repo update
helm upgrade --install aws-load-balancer-controller eks/aws-load-balancer-controller \
  -n kube-system \
  --set clusterName=lbcdemo-cluster \
  --set serviceAccount.create=true \
  --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"=arn:aws:iam::587472608760:role/lbcdemo-github-actions \
  --set region=us-east-1 \
  --set vpcId=vpc-085c640de456cb61f \
  --wait

# External Secrets Operator
helm repo add external-secrets https://charts.external-secrets.io && helm repo update
helm upgrade --install external-secrets external-secrets/external-secrets \
  -n kube-system --set installCRDs=true --wait
```

### 7. Deploy — push to main
```bash
git push origin main
```
This triggers `.github/workflows/deploy.yml` which:
1. Runs all tests
2. Builds and pushes Docker images to ECR (tagged `sha-<8chars>`)
3. Runs Trivy security scan
4. Deploys API + MLflow via Helm to EKS
5. Syncs frontend build to S3 + invalidates CloudFront cache

## Day-to-day Operations

### Redeploy after code changes
Just push to `main` — GitHub Actions handles everything.

### Check running pods
```bash
kubectl get pods -n lbcdemo
```

### View API logs
```bash
kubectl logs -n lbcdemo -l app=api --tail=100 -f
```

### Manual Helm deploy (without CI)
```bash
IMAGE_TAG=sha-<commit>
helm upgrade --install api helm/api -n lbcdemo \
  --set image.tag=$IMAGE_TAG \
  --set image.repository=587472608760.dkr.ecr.us-east-1.amazonaws.com/lbcdemo-api
```

### Access MLflow UI (port-forward)
```bash
kubectl port-forward -n lbcdemo svc/mlflow 5000:5000
# then open http://localhost:5000
```

### Access Grafana (port-forward)
```bash
kubectl port-forward -n lbcdemo svc/kube-prometheus-stack-grafana 3000:80
# then open http://localhost:3000 (default: admin/prom-operator)
```

## Teardown
```bash
cd infra/terraform
terraform destroy -auto-approve
```
> This destroys all AWS resources including RDS (data loss). ECR images and S3 objects are deleted because `force_destroy = true`.
