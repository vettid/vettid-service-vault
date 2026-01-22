# Service Vault AWS Architecture

## Overview

This document describes the AWS architecture for deploying Service Vaults at scale, supporting both self-hosted deployments and a VettID-managed multi-tenant option for smaller services.

## Design Goals

1. **Scale**: Support services with tens of users to hundreds of millions
2. **Cost**: Minimize costs, especially for small services, without compromising security
3. **Multi-tenancy**: Enable secure shared infrastructure for smaller services
4. **Security**: Strict isolation between tenants and services

---

## 1. Deployment Models

### 1.1 Self-Hosted (Dedicated Infrastructure)

For services that want full control or have regulatory requirements:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    SERVICE PROVIDER'S AWS ACCOUNT                            │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         Service Vault                                │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │    │
│  │  │ ECS/Fargate │  │  DynamoDB   │  │    NATS     │                 │    │
│  │  │  (compute)  │  │  (storage)  │  │(ServiceSpace)│                 │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                 │    │
│  │                                                                     │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │    │
│  │  │  KMS Keys   │  │   Secrets   │  │ CloudWatch  │                 │    │
│  │  │  (HSM-backed)│  │   Manager  │  │  (logging)  │                 │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      │                                       │
└──────────────────────────────────────┼──────────────────────────────────────┘
                                       │ TLS
                                       ▼
                         ┌────────────────────────┐
                         │  VettID MessageSpace   │
                         │  (VettID-hosted NATS)  │
                         └────────────────────────┘
```

**Best for:**
- Large services (millions of users)
- Regulated industries (healthcare, finance)
- Services requiring data residency
- Full infrastructure control

**Estimated cost:** $500-5,000+/month depending on scale

### 1.2 VettID-Managed Multi-Tenant (Shared Infrastructure)

For smaller services that want low cost and operational simplicity:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      VETTID MANAGED SERVICE VAULT                            │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    Shared Control Plane                              │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │    │
│  │  │  Tenant     │  │  Billing &  │  │  Monitoring │                 │    │
│  │  │  Onboarding │  │  Metering   │  │  & Alerts   │                 │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      │                                       │
│                          ┌───────────┴───────────┐                          │
│                          ▼                       ▼                          │
│  ┌─────────────────────────────┐  ┌─────────────────────────────┐          │
│  │      Pool Tier              │  │      Silo Tier              │          │
│  │   (Small Services)          │  │   (Large Services)          │          │
│  │                             │  │                             │          │
│  │  ┌───────────────────────┐  │  │  ┌───────────────────────┐  │          │
│  │  │ Shared ECS Cluster    │  │  │  │ Dedicated ECS Cluster │  │          │
│  │  │ (Fargate Spot)        │  │  │  │ (Reserved Capacity)   │  │          │
│  │  └───────────────────────┘  │  │  └───────────────────────┘  │          │
│  │                             │  │                             │          │
│  │  ┌───────────────────────┐  │  │  ┌───────────────────────┐  │          │
│  │  │ Shared DynamoDB       │  │  │  │ Dedicated DynamoDB    │  │          │
│  │  │ (tenant_id partition) │  │  │  │ Tables                │  │          │
│  │  └───────────────────────┘  │  │  └───────────────────────┘  │          │
│  │                             │  │                             │          │
│  │  ┌───────────────────────┐  │  │  ┌───────────────────────┐  │          │
│  │  │ Shared NATS Cluster   │  │  │  │ Dedicated NATS        │  │          │
│  │  │ (Account Isolation)   │  │  │  │ Account + Limits      │  │          │
│  │  └───────────────────────┘  │  │  └───────────────────────┘  │          │
│  │                             │  │                             │          │
│  │  Service A ────┐            │  │  Service X (isolated)       │          │
│  │  Service B ────┼─ isolated  │  │  Service Y (isolated)       │          │
│  │  Service C ────┘  by tenant │  │                             │          │
│  │                             │  │                             │          │
│  └─────────────────────────────┘  └─────────────────────────────┘          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Best for:**
- Small to medium services (tens to millions of users)
- Startups and indie developers
- Services wanting minimal operational overhead
- Cost-sensitive deployments

**Estimated cost:** $10-500/month depending on usage

---

## 2. Multi-Tenant Architecture (Pool Tier)

### 2.1 Tenant Isolation Model

```
                    ┌─────────────────────────────┐
                    │      API Gateway            │
                    │   (shared, rate-limited)    │
                    └─────────────┬───────────────┘
                                  │
                                  ▼
                    ┌─────────────────────────────┐
                    │   Lambda Authorizer         │
                    │   (validates service JWT,   │
                    │    extracts service_id)     │
                    └─────────────┬───────────────┘
                                  │
                                  ▼
                    ┌─────────────────────────────┐
                    │   Tenant Context Injection  │
                    │   service_id added to all   │
                    │   requests and queries      │
                    └─────────────┬───────────────┘
                                  │
            ┌─────────────────────┼─────────────────────┐
            ▼                     ▼                     ▼
   ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
   │    DynamoDB     │  │      NATS       │  │      KMS        │
   │                 │  │                 │  │                 │
   │ PK: service_id  │  │ Account per     │  │ Shared key with │
   │ + resource_id   │  │ service_id      │  │ context policy  │
   │                 │  │                 │  │                 │
   │ All queries     │  │ Topics scoped:  │  │ Encryption ops  │
   │ MUST include    │  │ ServiceSpace.   │  │ include         │
   │ service_id      │  │ <service_id>.>  │  │ service_id      │
   └─────────────────┘  └─────────────────┘  └─────────────────┘
```

### 2.2 DynamoDB Multi-Tenant Design

**Table: ServiceContracts**
```
Partition Key: service_id
Sort Key: contract_id

Attributes:
- service_id (String) - REQUIRED for all queries
- contract_id (String)
- user_id (String)
- offering_snapshot (Map)
- status (String)
- created_at (String)
- user_signature (Map)
- service_signature (Map)

GSI: UserContracts
- Partition Key: user_id
- Sort Key: service_id
- Projects: contract_id, status, created_at
```

**Table: ServiceConnections**
```
Partition Key: service_id
Sort Key: user_id

Attributes:
- service_id (String)
- user_id (String)
- connection_key_public (String)
- nats_credentials (Map) - encrypted
- last_activity (String)
- status (String)
```

**Tenant Isolation Enforcement:**
```typescript
// Middleware - ALL queries must include service_id
class TenantScopedRepository {
  constructor(private serviceId: string) {}

  async getContract(contractId: string): Promise<Contract> {
    // service_id is ALWAYS part of the query
    return dynamodb.get({
      TableName: 'ServiceContracts',
      Key: {
        service_id: this.serviceId,  // ENFORCED
        contract_id: contractId
      }
    });
  }

  async listContracts(): Promise<Contract[]> {
    // Query is automatically scoped to partition
    return dynamodb.query({
      TableName: 'ServiceContracts',
      KeyConditionExpression: 'service_id = :sid',
      ExpressionAttributeValues: {
        ':sid': this.serviceId  // ENFORCED
      }
    });
  }
}
```

### 2.3 NATS Multi-Tenant Isolation

Each service gets a dedicated NATS account, even in the shared cluster:

```
# NATS Server Configuration
accounts {
  # Pool tier services - shared cluster, isolated accounts

  service_abc123 {
    users: [
      { nkey: "SUAM..." }  # Service's signing key
    ]

    # Each service can only pub/sub to their own namespace
    exports: [
      { service: "ServiceSpace.abc123.>" }
    ]

    # Rate limits prevent noisy neighbor
    limits {
      max_connections: 100
      max_payload: 1048576      # 1MB
      max_subscriptions: 1000
      max_msgs_per_sec: 10000
      max_bytes_per_sec: 52428800  # 50MB/s
    }
  }

  service_def456 {
    users: [
      { nkey: "SUBM..." }
    ]
    exports: [
      { service: "ServiceSpace.def456.>" }
    ]
    limits {
      max_connections: 100
      # ... same limits
    }
  }
}
```

**Topic Structure:**
```
ServiceSpace.<service_id>.fromUser.<user_id>.>   # User → Service
ServiceSpace.<service_id>.toUser.<user_id>.>     # Service → User (setup only)
ServiceSpace.<service_id>.internal.>             # Service internal
```

### 2.4 Security Boundaries

| Layer | Isolation Mechanism |
|-------|---------------------|
| **API Gateway** | JWT validation, rate limiting per service_id |
| **Lambda/ECS** | service_id injected into context, all operations scoped |
| **DynamoDB** | Partition key includes service_id, queries enforced |
| **NATS** | Account isolation, topics namespaced by service_id |
| **KMS** | Encryption context includes service_id |
| **Secrets Manager** | Secrets namespaced: `/services/{service_id}/...` |
| **CloudWatch** | Logs include service_id dimension |

---

## 3. Infrastructure Components

### 3.1 Compute Layer (AWS Fargate)

```typescript
// CDK Configuration
const cluster = new ecs.Cluster(this, 'ServiceVaultCluster', {
  vpc,
  containerInsights: true,
});

const taskDefinition = new ecs.FargateTaskDefinition(this, 'ServiceVaultTask', {
  memoryLimitMiB: 512,
  cpu: 256,
  runtimePlatform: {
    cpuArchitecture: ecs.CpuArchitecture.ARM64,  // Cost optimization
    operatingSystemFamily: ecs.OperatingSystemFamily.LINUX,
  },
});

const service = new ecs.FargateService(this, 'ServiceVaultService', {
  cluster,
  taskDefinition,
  desiredCount: 2,
  capacityProviderStrategies: [
    {
      capacityProvider: 'FARGATE_SPOT',
      weight: 80,  // 80% spot for cost savings
    },
    {
      capacityProvider: 'FARGATE',
      weight: 20,  // 20% on-demand for stability
    },
  ],
});
```

### 3.2 Database Layer (DynamoDB)

```typescript
// Shared tables with PAY_PER_REQUEST for cost efficiency
const contractsTable = new dynamodb.Table(this, 'ServiceContracts', {
  tableName: 'service-vault-contracts',
  partitionKey: { name: 'service_id', type: dynamodb.AttributeType.STRING },
  sortKey: { name: 'contract_id', type: dynamodb.AttributeType.STRING },
  billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
  encryption: dynamodb.TableEncryption.CUSTOMER_MANAGED,
  encryptionKey: kmsKey,
  pointInTimeRecovery: true,
  removalPolicy: RemovalPolicy.RETAIN,
});

// GSI for user lookups
contractsTable.addGlobalSecondaryIndex({
  indexName: 'UserContracts',
  partitionKey: { name: 'user_id', type: dynamodb.AttributeType.STRING },
  sortKey: { name: 'service_id', type: dynamodb.AttributeType.STRING },
  projectionType: dynamodb.ProjectionType.INCLUDE,
  nonKeyAttributes: ['contract_id', 'status', 'created_at'],
});
```

**Cost Estimate (PAY_PER_REQUEST):**
| Usage Level | Reads/Month | Writes/Month | Est. Cost |
|-------------|-------------|--------------|-----------|
| Small (10 users) | 10K | 1K | ~$0.50 |
| Medium (10K users) | 10M | 1M | ~$15 |
| Large (1M users) | 1B | 100M | ~$500 |

### 3.3 NATS Cluster

```typescript
// NATS deployed on EC2 for cost efficiency
const natsAsg = new autoscaling.AutoScalingGroup(this, 'NatsCluster', {
  vpc,
  instanceType: ec2.InstanceType.of(ec2.InstanceClass.T4G, ec2.InstanceSize.SMALL),
  machineImage: new ec2.AmazonLinuxImage({
    generation: ec2.AmazonLinuxGeneration.AMAZON_LINUX_2023,
    cpuType: ec2.AmazonLinuxCpuType.ARM_64,
  }),
  minCapacity: 3,
  maxCapacity: 9,
  desiredCapacity: 3,
});

// NLB for external access
const nlb = new elbv2.NetworkLoadBalancer(this, 'NatsNlb', {
  vpc,
  internetFacing: false,  // Internal only
  crossZoneEnabled: true,
});
```

**Cost Estimate:**
| Configuration | Instances | Est. Cost/Month |
|---------------|-----------|-----------------|
| Minimal | 3x t4g.micro | ~$25 |
| Standard | 3x t4g.small | ~$50 |
| High Availability | 3x t4g.medium | ~$100 |

### 3.4 Security Components

**KMS Key for Multi-Tenant Encryption:**
```typescript
const serviceVaultKey = new kms.Key(this, 'ServiceVaultKey', {
  description: 'Encrypts service vault data',
  enableKeyRotation: true,
  policy: new iam.PolicyDocument({
    statements: [
      new iam.PolicyStatement({
        effect: iam.Effect.ALLOW,
        principals: [new iam.ServicePrincipal('dynamodb.amazonaws.com')],
        actions: ['kms:Encrypt', 'kms:Decrypt', 'kms:GenerateDataKey*'],
        resources: ['*'],
        conditions: {
          StringEquals: {
            'kms:ViaService': `dynamodb.${this.region}.amazonaws.com`,
            'kms:CallerAccount': this.account,
          },
        },
      }),
    ],
  }),
});
```

**Secrets Manager for Service Credentials:**
```typescript
// Per-service secrets with strict IAM policies
const createServiceSecret = (serviceId: string) => {
  return new secretsmanager.Secret(this, `Secret-${serviceId}`, {
    secretName: `/services/${serviceId}/nats-credentials`,
    generateSecretString: {
      secretStringTemplate: JSON.stringify({ service_id: serviceId }),
      generateStringKey: 'nats_seed',
    },
  });
};
```

---

## 4. Scaling Architecture

### 4.1 Pool Tier Scaling (Tens to Millions of Users)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        POOL TIER AUTO-SCALING                                │
│                                                                              │
│  API Gateway (auto-scales, no config needed)                                 │
│       │                                                                      │
│       ▼                                                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Application Load Balancer                                           │    │
│  │  - Target tracking: RequestCount per target                         │    │
│  └────────────────────────────────┬────────────────────────────────────┘    │
│                                   │                                          │
│                                   ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  ECS Fargate Service                                                 │    │
│  │  - Min: 2, Max: 100 tasks                                           │    │
│  │  - Target tracking: CPU 70%, Memory 80%                             │    │
│  │  - 80% Fargate Spot, 20% On-Demand                                  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                   │                                          │
│                                   ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  DynamoDB (PAY_PER_REQUEST)                                          │    │
│  │  - Auto-scales read/write capacity                                   │    │
│  │  - DAX cache for hot data (optional)                                │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                   │                                          │
│                                   ▼                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  NATS Cluster                                                        │    │
│  │  - ASG: Min 3, Max 9 nodes                                          │    │
│  │  - Scale on: Connection count, message throughput                   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Silo Tier Scaling (Dedicated Resources)

For large services that need dedicated infrastructure:

```typescript
// Dedicated infrastructure per service
interface SiloDeployment {
  serviceId: string;

  // Dedicated DynamoDB tables
  tables: {
    contracts: dynamodb.Table;
    connections: dynamodb.Table;
    audit: dynamodb.Table;
  };

  // Dedicated ECS service
  ecsService: ecs.FargateService;

  // Dedicated NATS account (or separate cluster)
  natsAccount: NatsAccount;

  // Dedicated KMS key
  kmsKey: kms.Key;
}

// Provisioned via CDK pipeline on tenant signup
```

### 4.3 Tier Migration Path

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         TIER MIGRATION PATH                                  │
│                                                                              │
│  Free Tier ────────────► Pro Tier ────────────► Enterprise Tier             │
│                                                                              │
│  Pool Model              Bridge Model           Silo Model                   │
│  - Shared compute        - Shared compute       - Dedicated compute          │
│  - Shared DB (RLS)       - Dedicated schema     - Dedicated DB               │
│  - Shared NATS acct      - Dedicated NATS acct  - Dedicated NATS cluster     │
│  - Basic rate limits     - Higher limits        - No shared limits           │
│                                                                              │
│  $10-50/mo               $100-500/mo            $500+/mo                     │
│                                                                              │
│  Migration Process:                                                          │
│  1. Provision new resources (schema/tables/cluster)                         │
│  2. Replicate data from pool                                                │
│  3. Update routing to new resources                                          │
│  4. Remove from pool (retain for rollback)                                  │
│  5. Cleanup pool data after grace period                                    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 5. Cost Optimization

### 5.1 Cost by Tier

| Component | Pool (Small) | Pool (Medium) | Silo (Large) |
|-----------|--------------|---------------|--------------|
| **Compute (Fargate)** | $5 (shared) | $20 (shared) | $200 (dedicated) |
| **DynamoDB** | $0.50 | $15 | $100+ |
| **NATS (shared)** | $2 (shared) | $5 (shared) | $50 (dedicated) |
| **API Gateway** | $1 | $10 | $50 |
| **KMS** | $1 | $1 | $10 |
| **CloudWatch** | $1 | $5 | $20 |
| **Data Transfer** | $1 | $10 | $100 |
| **Total** | **~$12/mo** | **~$70/mo** | **~$530/mo** |

### 5.2 Cost Optimization Techniques

1. **Fargate Spot (80% savings)**
   - Use for pool tier workloads
   - 20% on-demand for stability

2. **ARM64 Architecture (20% savings)**
   - Graviton instances for EC2
   - ARM64 for Lambda and Fargate

3. **PAY_PER_REQUEST DynamoDB**
   - No capacity planning
   - Ideal for variable workloads

4. **Reserved Capacity for Silo**
   - 1-year reserved for predictable workloads
   - 30-40% savings vs on-demand

5. **CloudWatch Log Retention**
   - 30 days for operational logs
   - 1 year for audit logs (S3 lifecycle to Glacier)

### 5.3 Usage-Based Pricing Model

```typescript
interface ServiceUsageMetrics {
  service_id: string;
  period: string;  // "2024-01"

  // Metered usage
  api_requests: number;
  nats_messages: number;
  storage_bytes: number;
  active_connections: number;

  // Computed cost
  compute_cost: number;
  storage_cost: number;
  network_cost: number;
  total_cost: number;
}

// CloudWatch metrics → Cost calculation
const calculateCost = (metrics: ServiceUsageMetrics): number => {
  return (
    (metrics.api_requests * 0.000001) +      // $1 per million requests
    (metrics.nats_messages * 0.0000001) +    // $0.10 per million messages
    (metrics.storage_bytes / 1e9 * 0.25) +   // $0.25 per GB
    (metrics.active_connections * 0.001)      // $0.001 per connection/day
  );
};
```

---

## 6. Security Architecture

### 6.1 Network Security

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          VPC ARCHITECTURE                                    │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                      Public Subnets                                  │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │    │
│  │  │    NAT GW   │  │     ALB     │  │     NLB     │                 │    │
│  │  │   (egress)  │  │  (HTTPS)    │  │   (NATS)    │                 │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      │                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                      Private Subnets                                 │    │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │    │
│  │  │   Fargate   │  │    NATS     │  │  DynamoDB   │                 │    │
│  │  │   Tasks     │  │   Cluster   │  │  Endpoint   │                 │    │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                 │    │
│  │                                                                     │    │
│  │  Security Groups:                                                   │    │
│  │  - fargate-sg: ingress 443 from ALB only                           │    │
│  │  - nats-sg: ingress 4222 from fargate-sg and NLB                   │    │
│  │  - No direct internet access from private subnets                  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  VPC Endpoints (no internet required):                                      │
│  - com.amazonaws.{region}.dynamodb                                          │
│  - com.amazonaws.{region}.secretsmanager                                    │
│  - com.amazonaws.{region}.kms                                               │
│  - com.amazonaws.{region}.logs                                              │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 IAM Least Privilege

```typescript
// Service Vault task role - minimal permissions
const taskRole = new iam.Role(this, 'ServiceVaultTaskRole', {
  assumedBy: new iam.ServicePrincipal('ecs-tasks.amazonaws.com'),
});

// DynamoDB access - scoped to service_id via conditions
taskRole.addToPolicy(new iam.PolicyStatement({
  effect: iam.Effect.ALLOW,
  actions: [
    'dynamodb:GetItem',
    'dynamodb:PutItem',
    'dynamodb:UpdateItem',
    'dynamodb:DeleteItem',
    'dynamodb:Query',
  ],
  resources: [
    contractsTable.tableArn,
    `${contractsTable.tableArn}/index/*`,
  ],
  conditions: {
    'ForAllValues:StringEquals': {
      'dynamodb:LeadingKeys': ['${aws:PrincipalTag/service_id}'],
    },
  },
}));

// Secrets access - only own service's secrets
taskRole.addToPolicy(new iam.PolicyStatement({
  effect: iam.Effect.ALLOW,
  actions: ['secretsmanager:GetSecretValue'],
  resources: [
    `arn:aws:secretsmanager:${this.region}:${this.account}:secret:/services/\${aws:PrincipalTag/service_id}/*`,
  ],
}));
```

### 6.3 Audit Logging

```typescript
// CloudTrail for API audit
const trail = new cloudtrail.Trail(this, 'ServiceVaultTrail', {
  bucket: auditBucket,
  includeGlobalServiceEvents: true,
  isMultiRegionTrail: false,
  enableFileValidation: true,
});

// DynamoDB Streams for data changes
contractsTable.enableStream({
  streamViewType: dynamodb.StreamViewType.NEW_AND_OLD_IMAGES,
});

// Stream processor logs all changes
const auditProcessor = new lambda.Function(this, 'AuditProcessor', {
  // Logs service_id, action, timestamp, before/after
});

auditProcessor.addEventSource(new DynamoDBEventSource(contractsTable, {
  startingPosition: lambda.StartingPosition.LATEST,
  batchSize: 100,
}));
```

---

## 7. Deployment Architecture

### 7.1 CDK Stack Structure

```
service-vault-cdk/
├── bin/
│   └── app.ts                    # CDK app entry point
├── lib/
│   ├── stacks/
│   │   ├── network-stack.ts      # VPC, subnets, endpoints
│   │   ├── security-stack.ts     # KMS, IAM, secrets
│   │   ├── database-stack.ts     # DynamoDB tables
│   │   ├── nats-stack.ts         # NATS cluster
│   │   ├── compute-stack.ts      # ECS/Fargate
│   │   ├── api-stack.ts          # API Gateway, Lambda
│   │   └── monitoring-stack.ts   # CloudWatch, alarms
│   └── constructs/
│       ├── tenant-isolation.ts   # Multi-tenant constructs
│       └── service-vault.ts      # Service vault construct
└── test/
    └── *.test.ts
```

### 7.2 CI/CD Pipeline

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         CI/CD PIPELINE                                       │
│                                                                              │
│  GitHub Push ──► CodePipeline ──► CodeBuild ──► CDK Deploy                  │
│                                                                              │
│  Stages:                                                                     │
│  1. Source (GitHub)                                                          │
│  2. Build & Test (CodeBuild)                                                │
│  3. Deploy to Dev (CDK)                                                      │
│  4. Integration Tests                                                        │
│  5. Manual Approval                                                          │
│  6. Deploy to Prod (CDK)                                                     │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 8. Monitoring & Observability

### 8.1 Key Metrics

| Metric | Description | Alarm Threshold |
|--------|-------------|-----------------|
| `ApiLatencyP99` | API response time (p99) | > 1000ms |
| `ApiErrorRate` | 5xx error rate | > 1% |
| `NatsConnectionCount` | Active NATS connections | > 80% capacity |
| `NatsMessageRate` | Messages per second | > 90% limit |
| `DynamoDBThrottles` | Throttled requests | > 0 |
| `TenantApiCalls` | Calls per tenant | For billing |

### 8.2 CloudWatch Dashboard

```typescript
const dashboard = new cloudwatch.Dashboard(this, 'ServiceVaultDashboard', {
  dashboardName: 'service-vault-operations',
  widgets: [
    [
      new cloudwatch.GraphWidget({
        title: 'API Latency',
        left: [apiLatencyMetric],
      }),
      new cloudwatch.GraphWidget({
        title: 'Error Rate',
        left: [errorRateMetric],
      }),
    ],
    [
      new cloudwatch.GraphWidget({
        title: 'NATS Connections by Service',
        left: [natsConnectionsMetric],
      }),
      new cloudwatch.GraphWidget({
        title: 'DynamoDB Usage',
        left: [readCapacityMetric, writeCapacityMetric],
      }),
    ],
  ],
});
```

---

## 9. Implementation Phases

### Phase 1: Foundation (Week 1-2)
- [ ] VPC and networking
- [ ] KMS keys and secrets
- [ ] DynamoDB tables
- [ ] Basic ECS service

### Phase 2: NATS Integration (Week 3-4)
- [ ] NATS cluster deployment
- [ ] Account-based isolation
- [ ] MessageSpace integration

### Phase 3: Multi-Tenancy (Week 5-6)
- [ ] Tenant onboarding API
- [ ] Billing and metering
- [ ] Rate limiting

### Phase 4: Scaling & Optimization (Week 7-8)
- [ ] Auto-scaling policies
- [ ] Cost optimization
- [ ] Performance testing

### Phase 5: Production Readiness (Week 9-10)
- [ ] Monitoring and alerting
- [ ] Documentation
- [ ] Security review

---

## 10. Decision Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Deployment Model** | Hybrid (Pool + Silo) | Supports all service sizes cost-effectively |
| **Compute** | Fargate (ARM64) | Serverless scaling, cost-efficient |
| **Database** | DynamoDB PAY_PER_REQUEST | Auto-scaling, no capacity planning |
| **Messaging** | NATS with account isolation | Existing VettID pattern, strong isolation |
| **Multi-tenancy** | Partition key + NATS accounts | Simple, secure, scalable |
| **Encryption** | KMS customer-managed | Compliance, audit trail |
| **No Nitro Enclave** | Not needed | Service vault doesn't hold user secrets |

---

## 11. Review Checklist

### 11.1 Security Review

**Multi-Tenant Isolation**
- [ ] NATS account isolation: Can a compromised service access another service's topics?
- [ ] DynamoDB partition key enforcement: Are there any query paths that bypass `service_id` scoping?
- [ ] IAM condition policies: Can `${aws:PrincipalTag/service_id}` be spoofed or bypassed?
- [ ] Cross-tenant data leakage: Review GSI queries (UserContracts) for isolation gaps
- [ ] Secrets Manager namespacing: Verify `/services/{service_id}/*` paths are enforced

**Network Security**
- [ ] VPC endpoints: Confirm no internet egress required for AWS service calls
- [ ] Security groups: Validate minimal ingress rules (443 from ALB, 4222 from internal only)
- [ ] NLB exposure: Is NATS NLB correctly internal-only?
- [ ] TLS configuration: Verify TLS 1.3 enforcement on all endpoints

**Cryptography & Key Management**
- [ ] KMS key policy: Review encryption context enforcement
- [ ] Key rotation: Confirm automatic rotation is enabled
- [ ] Secrets lifecycle: How are NATS credentials rotated?

**Blast Radius Analysis**
- [ ] Pool tier compute compromise: What data can an attacker access?
- [ ] NATS cluster compromise: Can messages be intercepted across tenants?
- [ ] DynamoDB table access: If IAM is bypassed, what's the exposure?

**Compliance**
- [ ] Audit logging completeness: Are all data access operations logged?
- [ ] Data retention: How long is audit data retained?
- [ ] Right to deletion: Can a service's data be fully purged?

### 11.2 Architecture Review

**Scaling Assumptions**
- [ ] Fargate Spot 80/20 ratio: Is this appropriate for production workloads?
- [ ] NATS cluster sizing (3x t4g.small): Sufficient for projected message volume?
- [ ] DynamoDB PAY_PER_REQUEST: Cost-effective at scale, or should we use provisioned?

**Cost Estimates**
- [ ] Pool tier ($12/mo): Validate shared resource allocation model
- [ ] Silo tier ($530/mo): Verify dedicated resource costs
- [ ] Data transfer costs: Are cross-AZ and internet egress costs accounted for?

**Failure Modes**
- [ ] NATS cluster failure: What's the recovery process? Data loss implications?
- [ ] Fargate Spot interruption: Is 20% on-demand sufficient for continuity?
- [ ] DynamoDB throttling: How does the system behave under throttle?
- [ ] MessageSpace connectivity loss: How do services handle VettID NATS unavailability?

**Tier Migration**
- [ ] Pool → Silo migration: Is the data migration strategy defined?
- [ ] Rollback procedure: Can a failed migration be reversed?
- [ ] Zero-downtime migration: Is this achievable with the current design?

**Operational Concerns**
- [ ] Tenant onboarding automation: Is the process fully automated?
- [ ] Monitoring per-tenant: Can we alert on individual service health?
- [ ] Capacity planning: How do we know when to scale the pool tier?

### 11.3 Open Questions for Reviewers

1. **Shared vs Isolated NATS**: Should pool tier services share a NATS cluster, or get isolated EC2 instances per service?

2. **Table-level isolation threshold**: At what usage level should a service move from partition-key isolation to dedicated tables?

3. **Bridge tier**: Should there be an intermediate tier between pool and silo for medium-sized services?

4. **Multi-region**: What's the strategy for services requiring regional deployment?

5. **Service mesh**: Should we consider Istio/App Mesh for service-to-service communication within the vault?

6. **Backup strategy**: What's the RPO/RTO for service data? Is DynamoDB PITR sufficient?
