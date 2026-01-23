#!/usr/bin/env node
import * as cdk from 'aws-cdk-lib';
import { NetworkStack } from '../lib/stacks/network-stack';
import { SecurityStack } from '../lib/stacks/security-stack';
import { DatabaseStack } from '../lib/stacks/database-stack';
import { ComputeStack } from '../lib/stacks/compute-stack';
import { MonitoringStack } from '../lib/stacks/monitoring-stack';
import { ServiceRegistryStack } from '../lib/stacks/service-registry-stack';

const app = new cdk.App();

// Get tier from context (default to 'pool')
const tier = app.node.tryGetContext('tier') as 'pool' | 'silo' ?? 'pool';

// Environment configuration
const env = {
  account: process.env.CDK_DEFAULT_ACCOUNT,
  region: process.env.CDK_DEFAULT_REGION ?? 'us-east-1',
};

// Stack naming based on tier
const stackPrefix = tier === 'silo' ? 'VettIDSilo' : 'VettID';

// ============================================================================
// Network Stack - VPC, subnets, VPC endpoints
// ============================================================================
const networkStack = new NetworkStack(app, `${stackPrefix}Network`, {
  env,
  tier,
  description: 'VettID Service Vault network infrastructure',
});

// ============================================================================
// Security Stack - KMS, IAM roles
// ============================================================================
const securityStack = new SecurityStack(app, `${stackPrefix}Security`, {
  env,
  tier,
  description: 'VettID Service Vault security infrastructure',
});

// ============================================================================
// Database Stack - DynamoDB tables
// ============================================================================
const databaseStack = new DatabaseStack(app, `${stackPrefix}Database`, {
  env,
  tier,
  description: 'VettID Service Vault database infrastructure',
});

// Grant DynamoDB access to task role
securityStack.grantDynamoDbAccess([
  databaseStack.contractsTable.tableArn,
  databaseStack.requestsTable.tableArn,
]);

// ============================================================================
// Compute Stack - ECS/Fargate
// ============================================================================
const computeStack = new ComputeStack(app, `${stackPrefix}Compute`, {
  env,
  vpc: networkStack.vpc,
  securityGroup: networkStack.vaultSecurityGroup,
  taskRole: securityStack.vaultTaskRole,
  tier,
  contractsTableName: databaseStack.contractsTable.tableName,
  requestsTableName: databaseStack.requestsTable.tableName,
  description: 'VettID Service Vault compute infrastructure',
});
computeStack.addDependency(networkStack);
computeStack.addDependency(securityStack);
computeStack.addDependency(databaseStack);

// ============================================================================
// Monitoring Stack - CloudWatch dashboards, alarms
// ============================================================================
const monitoringStack = new MonitoringStack(app, `${stackPrefix}Monitoring`, {
  env,
  cluster: computeStack.cluster,
  service: computeStack.service,
  loadBalancer: computeStack.loadBalancer,
  contractsTable: databaseStack.contractsTable,
  requestsTable: databaseStack.requestsTable,
  tier,
  description: 'VettID Service Vault monitoring infrastructure',
});
monitoringStack.addDependency(computeStack);
monitoringStack.addDependency(databaseStack);

// ============================================================================
// Service Registry Stack - Service registration and discovery
// ============================================================================
const registryStack = new ServiceRegistryStack(app, `${stackPrefix}Registry`, {
  env,
  description: 'VettID Service Registry for service discovery',
});

// Add tags to all stacks
const stacks = [
  networkStack,
  securityStack,
  databaseStack,
  computeStack,
  monitoringStack,
  registryStack,
];

stacks.forEach(stack => {
  cdk.Tags.of(stack).add('Project', 'VettID');
  cdk.Tags.of(stack).add('Component', 'ServiceVault');
  cdk.Tags.of(stack).add('Tier', tier);
  cdk.Tags.of(stack).add('ManagedBy', 'CDK');
});
