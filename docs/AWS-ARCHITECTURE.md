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
- Small to medium services (tens to hundreds of thousands of users)
- Startups and indie developers
- Services wanting minimal operational overhead
- Cost-sensitive deployments

**Estimated cost:**
- Pool tier: $20-50/month (small services)
- Pro tier: $150-250/month (medium services)
- Silo tier: $600-1400/month (large services)

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
- Partition Key: user_id#service_id (composite key for tenant isolation)
- Sort Key: created_at
- Projects: contract_id, status
```

**Table: ServiceConnections**
```
Partition Key: service_id
Sort Key: user_id

Attributes:
- service_id (String)
- user_id (String)
- connection_key_public (String)
- nats_credentials_secret_arn (String) - reference to Secrets Manager
- last_activity (String)
- status (String)
```

> **Security Note**: NATS credentials are stored in Secrets Manager (not DynamoDB)
> at path `/services/{service_id}/nats-credentials`. The table stores only the
> secret ARN reference.

**Tenant Isolation Enforcement:**

> **Critical**: IAM conditions like `dynamodb:LeadingKeys` do NOT prevent Scan
> operations or GSI queries without partition keys. Application-level enforcement
> is mandatory.

```typescript
// Application-level enforcement - ALL queries must include service_id
class TenantScopedRepository {
  constructor(private serviceId: string) {
    if (!serviceId) {
      throw new Error('service_id is required for tenant isolation');
    }
  }

  // CRITICAL: Block Scan operations entirely
  async scan(): Promise<never> {
    throw new Error('Scan operations are forbidden on multi-tenant tables');
  }

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

  // GSI queries MUST include service_id in composite key
  async getContractsByUser(userId: string): Promise<Contract[]> {
    return dynamodb.query({
      TableName: 'ServiceContracts',
      IndexName: 'UserContracts',
      KeyConditionExpression: 'user_id_service_id = :composite',
      ExpressionAttributeValues: {
        ':composite': `${userId}#${this.serviceId}`  // ENFORCED
      }
    });
  }
}
```

### 2.3 NATS Multi-Tenant Isolation

Each service gets a dedicated NATS account, even in the shared cluster:

```
# NATS Server Configuration
jetstream {
  store_dir: /data/nats/jetstream
  max_memory_store: 1GB
  max_file_store: 100GB
}

accounts {
  # Pool tier services - shared cluster, isolated accounts

  service_abc123 {
    users: [
      { nkey: "SUAM..." }  # Service's signing key
    ]

    jetstream: enabled

    # CRITICAL: Explicit permissions prevent cross-tenant access
    permissions {
      publish {
        allow: ["ServiceSpace.abc123.>"]
        deny: ["ServiceSpace.*.>"]  # Deny other namespaces explicitly
      }
      subscribe {
        allow: ["ServiceSpace.abc123.>"]
        deny: ["ServiceSpace.*.>"]
      }
    }

    # CRITICAL: Disable imports to prevent cross-account access
    imports: []

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
    jetstream: enabled
    permissions {
      publish { allow: ["ServiceSpace.def456.>"] }
      subscribe { allow: ["ServiceSpace.def456.>"] }
    }
    imports: []
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

> **JetStream**: Enabled for message durability with R3 replication. Prevents
> data loss on cluster failure.

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
  // Extended stop timeout for graceful shutdown (drain NATS connections)
  stopTimeout: Duration.seconds(120),
  capacityProviderStrategies: [
    {
      capacityProvider: 'FARGATE_SPOT',
      weight: 60,  // 60% spot (reduced from 80% for stateful operations)
    },
    {
      capacityProvider: 'FARGATE',
      weight: 40,  // 40% on-demand for stability
      base: 2,     // Always keep 2 on-demand tasks running
    },
  ],
});
```

**Graceful Shutdown Handler:**
```typescript
// Application must handle SIGTERM for Spot interruptions
process.on('SIGTERM', async () => {
  logger.info('SIGTERM received, draining connections');

  // 1. Stop accepting new requests
  await server.close();

  // 2. Drain NATS connections (finish in-flight messages)
  await natsClient.drain();

  // 3. Complete pending DynamoDB writes
  await flushPendingWrites();

  logger.info('Graceful shutdown complete');
  process.exit(0);
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
// NATS deployed on EC2 with JetStream for message durability
const natsAsg = new autoscaling.AutoScalingGroup(this, 'NatsCluster', {
  vpc,
  vpcSubnets: { subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS },
  // t4g.medium recommended for production (4 vCPU, 8GB RAM)
  instanceType: ec2.InstanceType.of(ec2.InstanceClass.T4G, ec2.InstanceSize.MEDIUM),
  machineImage: new ec2.AmazonLinuxImage({
    generation: ec2.AmazonLinuxGeneration.AMAZON_LINUX_2023,
    cpuType: ec2.AmazonLinuxCpuType.ARM_64,
  }),
  minCapacity: 3,
  maxCapacity: 7,  // Beyond 7 nodes, use superclusters
  desiredCapacity: 3,
  // No SSH keys - use Session Manager for emergency access
  keyName: undefined,
  // EBS for JetStream persistence
  blockDevices: [{
    deviceName: '/dev/xvda',
    volume: autoscaling.BlockDeviceVolume.ebs(100, {
      volumeType: autoscaling.EbsDeviceVolumeType.GP3,
      encrypted: true,
    }),
  }],
});

// Session Manager access instead of SSH
natsAsg.role.addManagedPolicy(
  iam.ManagedPolicy.fromAwsManagedPolicyName('AmazonSSMManagedInstanceCore')
);

// NLB for internal access only
const nlb = new elbv2.NetworkLoadBalancer(this, 'NatsNlb', {
  vpc,
  internetFacing: false,  // Internal only
  crossZoneEnabled: true,
});
```

**TLS Configuration (Required):**
```conf
# NATS server.conf - TLS 1.3 enforcement
tls {
  cert_file: "/etc/nats/certs/server-cert.pem"
  key_file: "/etc/nats/certs/server-key.pem"
  ca_file: "/etc/nats/certs/ca.pem"
  min_version: "1.3"
  verify: true           # Require client certificates
  verify_and_map: true   # Map client cert to NATS user
}
```

**Cost Estimate:**
| Configuration | Instances | Est. Cost/Month |
|---------------|-----------|-----------------|
| Development | 3x t4g.small | ~$50 |
| Production | 3x t4g.medium | ~$100 |
| High Scale | 3x t4g.large + JetStream storage | ~$200 |

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

| Component | Pool | Pro | Silo |
|-----------|------|-----|------|
| **Compute (Fargate)** | $8-12 (shared) | $50 (dedicated task) | $150-250 (dedicated cluster) |
| **DynamoDB** | $0.50-5 | $20-50 (dedicated tables) | $80-200 |
| **NATS** | $3-5 (shared cluster) | $10 (dedicated account) | $50-100 (dedicated cluster) |
| **API Gateway** | $1-5 | $10-20 | $50-100 |
| **KMS** | $1 | $5 | $10 |
| **CloudWatch** | $2-5 | $10-20 | $30-50 |
| **Data Transfer** | $5-15 | $30-50 | $200-500 |
| **NAT Gateway** | (shared) | $32-64 | $96-128 |
| **Total** | **$20-50/mo** | **$150-250/mo** | **$600-1400/mo** |

> **Note**: Data transfer costs are often underestimated. Includes cross-AZ traffic,
> NAT Gateway, and internet egress. For high-volume services, consider VPC endpoints
> and PrivateLink to reduce costs.

### 5.2 Tier Comparison

| Feature | Pool | Pro | Silo |
|---------|------|-----|------|
| **Users** | 10-10K | 10K-100K | 100K+ |
| **Compute** | Shared tasks | Dedicated task | Dedicated cluster |
| **Database** | Shared table (partition key) | Dedicated tables (provisioned) | Dedicated tables (reserved) |
| **NATS** | Shared cluster (account isolation) | Shared cluster (higher limits) | Dedicated cluster |
| **Isolation** | Logical | Logical + resource | Physical |
| **SLA** | Best effort | 99.5% | 99.9% |

### 5.3 Cost Optimization Techniques

1. **Fargate Spot (40-60% savings)**
   - Use 60/40 Spot/On-Demand ratio for pool tier
   - Implement graceful shutdown handlers

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

**VPC Endpoint Policies (Required):**
```typescript
// DynamoDB endpoint - restrict to Service Vault tables only
const dynamoDBEndpoint = new ec2.GatewayVpcEndpoint(this, 'DynamoDBEndpoint', {
  vpc,
  service: ec2.GatewayVpcEndpointAwsService.DYNAMODB,
});

dynamoDBEndpoint.addToPolicy(new iam.PolicyStatement({
  effect: iam.Effect.ALLOW,
  principals: [new iam.AnyPrincipal()],
  actions: ['dynamodb:*'],
  resources: [
    contractsTable.tableArn,
    connectionsTable.tableArn,
    `${contractsTable.tableArn}/index/*`,
    `${connectionsTable.tableArn}/index/*`,
  ],
  conditions: {
    StringEquals: { 'aws:PrincipalAccount': this.account }
  }
}));

// Secrets Manager endpoint - restrict to service paths
const secretsEndpoint = new ec2.InterfaceVpcEndpoint(this, 'SecretsEndpoint', {
  vpc,
  service: ec2.InterfaceVpcEndpointAwsService.SECRETS_MANAGER,
  privateDnsEnabled: true,
});

secretsEndpoint.addToPolicy(new iam.PolicyStatement({
  effect: iam.Effect.ALLOW,
  principals: [new iam.AnyPrincipal()],
  actions: ['secretsmanager:GetSecretValue'],
  resources: [`arn:aws:secretsmanager:${this.region}:${this.account}:secret:/services/*`],
}));
```

**ALB TLS Policy:**
```typescript
const listener = alb.addListener('HttpsListener', {
  port: 443,
  protocol: elbv2.ApplicationProtocol.HTTPS,
  certificates: [certificate],
  sslPolicy: elbv2.SslPolicy.TLS13_RES,  // TLS 1.3 only
});
```

### 6.2 IAM Least Privilege

> **Critical**: IAM `dynamodb:LeadingKeys` conditions do NOT prevent Scan operations
> or GSI queries without partition keys. Application-level enforcement (see Section 2.2)
> is the primary isolation mechanism. IAM provides defense-in-depth only.

```typescript
// Service Vault task role - minimal permissions
const taskRole = new iam.Role(this, 'ServiceVaultTaskRole', {
  assumedBy: new iam.ServicePrincipal('ecs-tasks.amazonaws.com'),
});

// DynamoDB access - explicitly deny Scan
taskRole.addToPolicy(new iam.PolicyStatement({
  effect: iam.Effect.DENY,
  actions: ['dynamodb:Scan'],  // CRITICAL: Block Scan operations
  resources: ['*'],
}));

// DynamoDB access - allow specific operations
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
    connectionsTable.tableArn,
    `${contractsTable.tableArn}/index/*`,
    `${connectionsTable.tableArn}/index/*`,
  ],
}));

// Secrets access - only own service's secrets
taskRole.addToPolicy(new iam.PolicyStatement({
  effect: iam.Effect.ALLOW,
  actions: ['secretsmanager:GetSecretValue'],
  resources: [
    `arn:aws:secretsmanager:${this.region}:${this.account}:secret:/services/*`,
  ],
}));

// Deny dangerous operations
taskRole.addToPolicy(new iam.PolicyStatement({
  effect: iam.Effect.DENY,
  actions: [
    'iam:*',           // Prevent role manipulation
    'sts:AssumeRole',  // Prevent role chaining
    'ec2:Describe*',   // Prevent enumeration
    'ecs:Describe*',
  ],
  resources: ['*'],
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

## 9. Backup & Disaster Recovery

### 9.1 RPO/RTO by Tier

| Tier | RPO | RTO | Method |
|------|-----|-----|--------|
| **Pool** | 5 minutes | 2-4 hours | DynamoDB PITR |
| **Pro** | 5 minutes | 1 hour | PITR + automated snapshots |
| **Silo** | 0 (continuous) | < 5 minutes | DynamoDB Global Tables |

### 9.2 Backup Strategy

**DynamoDB:**
```typescript
// Enable Point-in-Time Recovery for all tables
contractsTable.pointInTimeRecovery = true;
connectionsTable.pointInTimeRecovery = true;

// For Silo tier: Global Tables for cross-region replication
const siloTable = new dynamodb.Table(this, 'SiloContracts', {
  replicationRegions: ['us-west-2', 'eu-west-1'],
  pointInTimeRecovery: true,
});

// Automated backup plan
const backupPlan = new backup.BackupPlan(this, 'ServiceVaultBackup', {
  backupPlanRules: [
    backup.BackupPlanRule.daily(backupVault),
    backup.BackupPlanRule.weekly(backupVault, {
      moveToColdStorageAfter: Duration.days(90),
      deleteAfter: Duration.days(2555),  // 7 years for compliance
    }),
  ],
});

backupPlan.addSelection('DynamoDBTables', {
  resources: [
    backup.BackupResource.fromDynamoDbTable(contractsTable),
    backup.BackupResource.fromDynamoDbTable(connectionsTable),
  ],
});
```

**NATS JetStream:**
```typescript
// JetStream snapshots to S3
const jetStreamBackupLambda = new lambda.Function(this, 'JetStreamBackup', {
  runtime: lambda.Runtime.PROVIDED_AL2023,
  handler: 'bootstrap',
  code: lambda.Code.fromAsset('./lambda/jetstream-backup'),
  environment: {
    NATS_URL: natsCluster.endpoint,
    BACKUP_BUCKET: backupBucket.bucketName,
  },
});

// Schedule every 6 hours
new events.Rule(this, 'JetStreamBackupSchedule', {
  schedule: events.Schedule.rate(Duration.hours(6)),
  targets: [new targets.LambdaFunction(jetStreamBackupLambda)],
});
```

### 9.3 Disaster Recovery Procedures

**Pool Tier Recovery:**
1. Restore DynamoDB table from PITR (target time within 35 days)
2. Restart Fargate tasks (automatic via ECS service)
3. NATS cluster auto-recovers (JetStream replays from persistent storage)
4. Verify data consistency via audit logs

**Silo Tier Failover:**
1. Route53 health check detects primary region failure
2. Automatic DNS failover to secondary region
3. DynamoDB Global Tables provide read-your-writes consistency
4. Clients reconnect via new DNS endpoint (< 60 second TTL)

---

## 10. Implementation Phases

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

## 11. Decision Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Deployment Model** | Hybrid (Pool + Pro + Silo) | Supports all service sizes with smooth upgrade path |
| **Compute** | Fargate (ARM64, 60/40 Spot) | Serverless scaling, cost-efficient, resilient |
| **Database** | DynamoDB PAY_PER_REQUEST | Auto-scaling, no capacity planning |
| **Messaging** | NATS with JetStream + account isolation | Message durability, strong isolation |
| **Multi-tenancy** | Application-enforced partition keys + NATS accounts | Defense-in-depth (IAM alone insufficient) |
| **Encryption** | KMS customer-managed + TLS 1.3 | Compliance, audit trail, transport security |
| **Backup** | PITR (Pool/Pro), Global Tables (Silo) | RPO: 5min/0, RTO: hours/minutes |
| **No Nitro Enclave** | Not needed | Service vault doesn't hold user secrets |

---

## 12. Review Checklist

### 12.1 Security Review

**Multi-Tenant Isolation**
- [x] NATS account isolation: Explicit permissions and imports disabled (Section 2.3)
- [x] DynamoDB partition key enforcement: Application-level enforcement required (Section 2.2)
- [x] IAM condition policies: Documented as defense-in-depth only, not primary isolation (Section 6.2)
- [x] Cross-tenant data leakage: GSI uses composite key `user_id#service_id` (Section 2.2)
- [x] Secrets Manager namespacing: NATS credentials stored in Secrets Manager, not DynamoDB (Section 2.2)

**Network Security**
- [x] VPC endpoints: Restrictive endpoint policies added (Section 6.1)
- [x] Security groups: Documented minimal ingress rules (Section 6.1)
- [x] NLB exposure: Internal-only confirmed (Section 3.3)
- [x] TLS configuration: TLS 1.3 specified for ALB, NLB, NATS (Sections 3.3, 6.1)

**Cryptography & Key Management**
- [ ] KMS key policy: Review encryption context enforcement
- [x] Key rotation: Automatic rotation enabled
- [ ] Secrets lifecycle: NATS credential rotation strategy (document rotation Lambda)

**Blast Radius Analysis**
- [x] Pool tier compute compromise: IAM denies Scan, enumeration operations (Section 6.2)
- [x] NATS cluster compromise: JetStream enables message durability, TLS/mTLS for transport
- [x] DynamoDB table access: Application-level enforcement is primary control

**Compliance**
- [ ] Audit logging completeness: Are all data access operations logged?
- [x] Data retention: 7-year backup retention for compliance (Section 9.2)
- [ ] Right to deletion: Document data deletion workflow

### 12.2 Architecture Review

**Scaling Assumptions**
- [x] Fargate Spot ratio: Changed to 60/40 with graceful shutdown (Section 3.1)
- [x] NATS cluster sizing: Upgraded to t4g.medium, added JetStream (Section 3.3)
- [x] DynamoDB billing: PAY_PER_REQUEST for Pool, provisioned for Pro/Silo (Section 5.1)

**Cost Estimates**
- [x] Pool tier: Revised to $20-50/mo including data transfer (Section 5.1)
- [x] Pro tier: Added at $150-250/mo (Section 5.1)
- [x] Silo tier: Revised to $600-1400/mo (Section 5.1)
- [x] Data transfer costs: Now accounted for in all tiers (Section 5.1)

**Failure Modes**
- [x] NATS cluster failure: JetStream provides message durability (Section 2.3)
- [x] Fargate Spot interruption: 40% on-demand + graceful shutdown (Section 3.1)
- [ ] DynamoDB throttling: Document retry strategy and DAX caching
- [ ] MessageSpace connectivity loss: Document local queueing fallback

**Tier Migration**
- [ ] Pool → Pro → Silo migration: Define detailed runbook
- [ ] Rollback procedure: Document bidirectional replication during migration
- [ ] Zero-downtime: Achievable with careful planning, not guaranteed

**Operational Concerns**
- [ ] Tenant onboarding automation: Define API specification
- [ ] Monitoring per-tenant: Document service_id metric dimensions
- [ ] Capacity planning: Define tier upgrade triggers

### 12.3 Resolved Questions

1. **Shared vs Isolated NATS**: Shared cluster with account isolation for Pool/Pro. NATS account isolation is cryptographically enforced via nkeys. Dedicated cluster for Silo tier only.

2. **Table-level isolation threshold**: Migrate to dedicated tables at >1000 RCU or >100GB storage. Pro tier uses dedicated tables with provisioned capacity.

3. **Bridge tier**: Added Pro tier at $150-250/mo with dedicated Fargate task, dedicated DynamoDB tables, shared NATS cluster with higher limits.

4. **Multi-region**: DynamoDB Global Tables for Silo tier. Pool/Pro remain single-region with PITR backup.

5. **Service mesh**: Not recommended. Unnecessary complexity for this architecture. NATS already provides message routing and observability.

6. **Backup strategy**: Defined in Section 9. Pool: PITR (RPO 5min, RTO 2-4hr). Pro: PITR + snapshots (RPO 5min, RTO 1hr). Silo: Global Tables (RPO 0, RTO <5min).
