#!/usr/bin/env node
import * as cdk from 'aws-cdk-lib/core';
import { ServiceRegistryStack } from '../lib/stacks/service-registry-stack';
import { NetworkStack } from '../lib/stacks/network-stack';
import { DatabaseStack } from '../lib/stacks/database-stack';

const app = new cdk.App();

// Get environment configuration
const env = {
  account: process.env.CDK_DEFAULT_ACCOUNT,
  region: process.env.CDK_DEFAULT_REGION || 'us-east-1',
};

// Deployment tier: pool, pro, or silo
const tier = app.node.tryGetContext('tier') || 'pool';

// Network stack (VPC, subnets, security groups)
const networkStack = new NetworkStack(app, 'VettIDNetworkStack', {
  env,
  description: 'VettID Service Vault - Network infrastructure',
});

// Database stack (DynamoDB tables for contracts)
const databaseStack = new DatabaseStack(app, 'VettIDDatabaseStack', {
  env,
  tier,
  description: 'VettID Service Vault - Database infrastructure',
});

// Service Registry stack (Lambda functions for registration/directory)
const registryStack = new ServiceRegistryStack(app, 'VettIDServiceRegistryStack', {
  env,
  description: 'VettID Service Vault - Service Registry',
});

// Add tags to all stacks
cdk.Tags.of(app).add('Project', 'VettID');
cdk.Tags.of(app).add('Component', 'ServiceVault');
cdk.Tags.of(app).add('ManagedBy', 'CDK');
