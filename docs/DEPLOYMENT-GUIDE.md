# VettID Service Vault Deployment Guide

This guide covers deploying the VettID Service Vault in various environments.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Configuration](#configuration)
3. [Local Development](#local-development)
4. [Docker Deployment](#docker-deployment)
5. [AWS Deployment](#aws-deployment)
6. [Production Checklist](#production-checklist)

---

## Prerequisites

### Required Software

- **Go 1.23+** for building the service
- **AWS CLI** configured with appropriate credentials
- **Docker** (optional, for containerized deployment)

### AWS Resources

For production deployment, you'll need:

| Resource | Purpose |
|----------|---------|
| DynamoDB Table | Contract and session storage |
| KMS Key | Service key encryption |
| Secrets Manager | Sensitive configuration |
| VPC | Network isolation |
| ECS/Fargate or EC2 | Compute |

---

## Configuration

The Service Vault is configured via environment variables and/or a configuration file.

### Environment Variables

```bash
# Service Identity
SERVICE_NAME="My Service"
SERVICE_TYPE="generic"  # generic, payment, identity, commerce, healthcare, finance
NATS_ENDPOINT="nats://nats.example.com:4222"
DOMAIN="example.com"  # Optional: for domain verification

# Key Storage
KEYSTORE_TYPE="kms"  # memory, file, kms
KMS_KEY_ARN="arn:aws:kms:us-east-1:123456789:key/abc-123"

# Storage
DYNAMODB_TABLE="vettid-contracts"
AWS_REGION="us-east-1"

# NATS Credentials
SERVICESPACE_CREDS_FILE="/path/to/servicespace.creds"
MESSAGESPACE_CREDS_FILE="/path/to/messagespace.creds"

# API Server
API_ADDRESS=":8080"
API_KEY="your-api-key"  # For service backend authentication

# Optional
LOG_LEVEL="info"  # debug, info, warn, error
```

### Configuration File (config.yaml)

```yaml
service:
  name: "My Service"
  type: "generic"
  nats_endpoint: "nats://nats.example.com:4222"
  domain: "example.com"

keystore:
  type: "kms"
  kms_key_arn: "arn:aws:kms:us-east-1:123456789:key/abc-123"

storage:
  dynamodb_table: "vettid-contracts"
  region: "us-east-1"

nats:
  servicespace_creds: "/path/to/servicespace.creds"
  messagespace_creds: "/path/to/messagespace.creds"

api:
  address: ":8080"
  api_key: "your-api-key"

logging:
  level: "info"
```

---

## Local Development

### Quick Start

1. **Build the service:**
   ```bash
   cd vault
   go build -o service-vault ./cmd/service-vault
   ```

2. **Start with minimal config (in-memory):**
   ```bash
   export SERVICE_NAME="Dev Service"
   export SERVICE_TYPE="generic"
   export KEYSTORE_TYPE="memory"
   export API_ADDRESS=":8080"

   ./service-vault
   ```

3. **Verify it's running:**
   ```bash
   curl http://localhost:8080/health
   ```

### Development with NATS

For local development with NATS:

1. **Start local NATS server:**
   ```bash
   docker run -p 4222:4222 -p 8222:8222 nats:latest
   ```

2. **Configure Service Vault:**
   ```bash
   export NATS_ENDPOINT="nats://localhost:4222"
   export SERVICESPACE_ENABLED="true"
   ```

### Development with DynamoDB Local

1. **Start DynamoDB Local:**
   ```bash
   docker run -p 8000:8000 amazon/dynamodb-local
   ```

2. **Create the contracts table:**
   ```bash
   aws dynamodb create-table \
     --endpoint-url http://localhost:8000 \
     --table-name vettid-contracts \
     --attribute-definitions \
       AttributeName=service_id,AttributeType=S \
       AttributeName=contract_id,AttributeType=S \
       AttributeName=user_id,AttributeType=S \
     --key-schema \
       AttributeName=service_id,KeyType=HASH \
       AttributeName=contract_id,KeyType=RANGE \
     --global-secondary-indexes \
       '[{"IndexName":"UserIndex","KeySchema":[{"AttributeName":"user_id","KeyType":"HASH"}],"Projection":{"ProjectionType":"ALL"}}]' \
     --billing-mode PAY_PER_REQUEST
   ```

3. **Configure Service Vault:**
   ```bash
   export DYNAMODB_TABLE="vettid-contracts"
   export DYNAMODB_ENDPOINT="http://localhost:8000"
   ```

---

## Docker Deployment

### Dockerfile

```dockerfile
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY vault/ .
RUN go build -o service-vault ./cmd/service-vault

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/service-vault .

EXPOSE 8080

ENTRYPOINT ["./service-vault"]
```

### Build and Run

```bash
# Build
docker build -t vettid-service-vault:latest .

# Run
docker run -p 8080:8080 \
  -e SERVICE_NAME="My Service" \
  -e KEYSTORE_TYPE="memory" \
  vettid-service-vault:latest
```

### Docker Compose (Development)

```yaml
version: '3.8'

services:
  service-vault:
    build: .
    ports:
      - "8080:8080"
    environment:
      - SERVICE_NAME=My Service
      - KEYSTORE_TYPE=memory
      - NATS_ENDPOINT=nats://nats:4222
      - DYNAMODB_ENDPOINT=http://dynamodb:8000
      - DYNAMODB_TABLE=vettid-contracts
    depends_on:
      - nats
      - dynamodb

  nats:
    image: nats:latest
    ports:
      - "4222:4222"

  dynamodb:
    image: amazon/dynamodb-local
    ports:
      - "8000:8000"
```

---

## AWS Deployment

### Architecture Overview

```
                    ┌─────────────┐
                    │   Route 53  │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │     ALB     │
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
        ┌─────▼─────┐ ┌────▼────┐ ┌─────▼─────┐
        │  Fargate  │ │ Fargate │ │  Fargate  │
        │  Task 1   │ │ Task 2  │ │  Task 3   │
        └─────┬─────┘ └────┬────┘ └─────┬─────┘
              │            │            │
              └────────────┼────────────┘
                           │
           ┌───────────────┼───────────────┐
           │               │               │
     ┌─────▼─────┐   ┌─────▼─────┐   ┌─────▼─────┐
     │ DynamoDB  │   │    KMS    │   │   NATS    │
     │           │   │           │   │ (External)│
     └───────────┘   └───────────┘   └───────────┘
```

### CDK Deployment

1. **Deploy infrastructure:**
   ```bash
   cd cdk
   npm install
   npx cdk deploy ServiceVaultStack
   ```

2. **Set required secrets in Secrets Manager:**
   ```bash
   aws secretsmanager create-secret \
     --name vettid/service-vault/api-key \
     --secret-string "your-api-key"
   ```

### Manual AWS Setup

#### 1. Create DynamoDB Table

```bash
aws dynamodb create-table \
  --table-name vettid-service-contracts \
  --attribute-definitions \
    AttributeName=service_id,AttributeType=S \
    AttributeName=contract_id,AttributeType=S \
    AttributeName=user_id,AttributeType=S \
    AttributeName=status,AttributeType=S \
  --key-schema \
    AttributeName=service_id,KeyType=HASH \
    AttributeName=contract_id,KeyType=RANGE \
  --global-secondary-indexes \
    '[
      {
        "IndexName":"UserIndex",
        "KeySchema":[{"AttributeName":"user_id","KeyType":"HASH"}],
        "Projection":{"ProjectionType":"ALL"}
      },
      {
        "IndexName":"StatusIndex",
        "KeySchema":[
          {"AttributeName":"service_id","KeyType":"HASH"},
          {"AttributeName":"status","KeyType":"RANGE"}
        ],
        "Projection":{"ProjectionType":"ALL"}
      }
    ]' \
  --billing-mode PAY_PER_REQUEST \
  --tags Key=Project,Value=VettID
```

#### 2. Create KMS Key

```bash
aws kms create-key \
  --description "VettID Service Vault signing key" \
  --key-usage SIGN_VERIFY \
  --key-spec ECC_NIST_P256 \
  --tags TagKey=Project,TagValue=VettID
```

#### 3. Create ECS Task Definition

```json
{
  "family": "vettid-service-vault",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "256",
  "memory": "512",
  "executionRoleArn": "arn:aws:iam::ACCOUNT:role/ecsTaskExecutionRole",
  "taskRoleArn": "arn:aws:iam::ACCOUNT:role/vettidServiceVaultRole",
  "containerDefinitions": [
    {
      "name": "service-vault",
      "image": "ACCOUNT.dkr.ecr.REGION.amazonaws.com/vettid-service-vault:latest",
      "portMappings": [
        {
          "containerPort": 8080,
          "protocol": "tcp"
        }
      ],
      "environment": [
        {"name": "SERVICE_NAME", "value": "My Service"},
        {"name": "KEYSTORE_TYPE", "value": "kms"},
        {"name": "DYNAMODB_TABLE", "value": "vettid-service-contracts"}
      ],
      "secrets": [
        {
          "name": "KMS_KEY_ARN",
          "valueFrom": "arn:aws:secretsmanager:REGION:ACCOUNT:secret:vettid/kms-key-arn"
        },
        {
          "name": "API_KEY",
          "valueFrom": "arn:aws:secretsmanager:REGION:ACCOUNT:secret:vettid/api-key"
        }
      ],
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": {
          "awslogs-group": "/ecs/vettid-service-vault",
          "awslogs-region": "REGION",
          "awslogs-stream-prefix": "ecs"
        }
      }
    }
  ]
}
```

---

## Production Checklist

### Security

- [ ] **KMS for key storage** - Never use memory or file keystore in production
- [ ] **API key authentication** - Protect all API endpoints
- [ ] **TLS termination** - Use HTTPS via ALB or CloudFront
- [ ] **VPC isolation** - Deploy in private subnets
- [ ] **IAM least privilege** - Minimal permissions for task role
- [ ] **Secrets Manager** - Store all secrets securely
- [ ] **WAF** - Consider AWS WAF for API protection

### Reliability

- [ ] **Multi-AZ deployment** - At least 2 Fargate tasks across AZs
- [ ] **Health checks** - Configure ALB health checks on `/health`
- [ ] **Auto-scaling** - Configure target tracking scaling
- [ ] **DynamoDB capacity** - Use on-demand or provisioned with auto-scaling

### Observability

- [ ] **CloudWatch Logs** - Enable container logging
- [ ] **CloudWatch Metrics** - Monitor API latency, error rates
- [ ] **X-Ray tracing** - Optional, for request tracing
- [ ] **Alarms** - Set up alarms for errors and latency

### Backup & Recovery

- [ ] **DynamoDB PITR** - Enable point-in-time recovery
- [ ] **Key rotation** - Configure KMS key rotation
- [ ] **Backup strategy** - Document and test recovery procedures

### Compliance

- [ ] **Audit logging** - Log all contract operations
- [ ] **Data retention** - Configure TTL for expired contracts
- [ ] **GDPR compliance** - Implement data deletion capabilities

---

## Troubleshooting

### Common Issues

#### Service won't start

1. Check environment variables are set correctly
2. Verify AWS credentials have required permissions
3. Check DynamoDB table exists and is accessible
4. Verify NATS credentials are valid

#### Contract operations failing

1. Check DynamoDB table has correct indexes
2. Verify service_id matches in all operations
3. Check contract status is valid for the operation

#### NATS connection issues

1. Verify NATS endpoint is reachable
2. Check credential files exist and are valid
3. Verify account permissions in NATS

### Debug Mode

Enable debug logging:

```bash
export LOG_LEVEL=debug
```

### Health Check Endpoint

```bash
curl http://localhost:8080/health

# Response:
{
  "status": "healthy",
  "version": "1.0.0",
  "components": {
    "dynamodb": "ok",
    "nats": "ok"
  }
}
```

---

## Next Steps

- [SDK Integration Guide](SDK-INTEGRATION-GUIDE.md) - Integrate with your backend
- [Security Model](SECURITY-MODEL.md) - Understand the security architecture
- [API Reference](api/openapi.yaml) - Full API documentation
