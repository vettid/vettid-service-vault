import * as cdk from 'aws-cdk-lib';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import { Construct } from 'constructs';

export interface DatabaseStackProps extends cdk.StackProps {
  readonly tier: 'pool' | 'silo';
}

/**
 * DatabaseStack creates DynamoDB tables for the VettID Service Vault.
 *
 * Table structure supports multi-tenant isolation with service_id as
 * a required part of all partition keys.
 */
export class DatabaseStack extends cdk.Stack {
  public readonly contractsTable: dynamodb.Table;
  public readonly requestsTable: dynamodb.Table;

  constructor(scope: Construct, id: string, props: DatabaseStackProps) {
    super(scope, id, props);

    const removalPolicy = props.tier === 'silo'
      ? cdk.RemovalPolicy.RETAIN
      : cdk.RemovalPolicy.DESTROY;

    // Contracts table
    // Primary key: service_id (PK), contract_id (SK)
    // GSI: user_id -> contracts for that user
    // Silo tier uses provisioned capacity with auto-scaling for predictable costs
    this.contractsTable = new dynamodb.Table(this, 'ContractsTable', {
      tableName: 'vettid-service-vault-contracts',
      partitionKey: {
        name: 'service_id',
        type: dynamodb.AttributeType.STRING
      },
      sortKey: {
        name: 'contract_id',
        type: dynamodb.AttributeType.STRING
      },
      billingMode: props.tier === 'silo'
        ? dynamodb.BillingMode.PROVISIONED
        : dynamodb.BillingMode.PAY_PER_REQUEST,
      readCapacity: props.tier === 'silo' ? 25 : undefined,
      writeCapacity: props.tier === 'silo' ? 25 : undefined,
      encryption: dynamodb.TableEncryption.AWS_MANAGED,
      pointInTimeRecoverySpecification: { pointInTimeRecoveryEnabled: true },
      removalPolicy,
    });

    // Enable auto-scaling for silo tier
    if (props.tier === 'silo') {
      const readScaling = this.contractsTable.autoScaleReadCapacity({
        minCapacity: 25,
        maxCapacity: 1000,
      });
      readScaling.scaleOnUtilization({
        targetUtilizationPercent: 70,
      });

      const writeScaling = this.contractsTable.autoScaleWriteCapacity({
        minCapacity: 25,
        maxCapacity: 1000,
      });
      writeScaling.scaleOnUtilization({
        targetUtilizationPercent: 70,
      });
    }

    // GSI: Look up contracts by user_id
    this.contractsTable.addGlobalSecondaryIndex({
      indexName: 'UserIndex',
      partitionKey: {
        name: 'user_id',
        type: dynamodb.AttributeType.STRING
      },
      sortKey: {
        name: 'created_at',
        type: dynamodb.AttributeType.STRING
      },
      projectionType: dynamodb.ProjectionType.ALL,
    });

    // GSI: Look up contracts by status for a service
    this.contractsTable.addGlobalSecondaryIndex({
      indexName: 'StatusIndex',
      partitionKey: {
        name: 'service_id',
        type: dynamodb.AttributeType.STRING
      },
      sortKey: {
        name: 'status',
        type: dynamodb.AttributeType.STRING
      },
      projectionType: dynamodb.ProjectionType.KEYS_ONLY,
    });

    // Auth/Authz requests table (short-lived)
    // Primary key: service_id (PK), request_id (SK)
    // TTL for automatic cleanup of expired requests
    this.requestsTable = new dynamodb.Table(this, 'RequestsTable', {
      tableName: 'vettid-service-vault-requests',
      partitionKey: {
        name: 'service_id',
        type: dynamodb.AttributeType.STRING
      },
      sortKey: {
        name: 'request_id',
        type: dynamodb.AttributeType.STRING
      },
      billingMode: props.tier === 'silo'
        ? dynamodb.BillingMode.PROVISIONED
        : dynamodb.BillingMode.PAY_PER_REQUEST,
      readCapacity: props.tier === 'silo' ? 50 : undefined,
      writeCapacity: props.tier === 'silo' ? 50 : undefined,
      encryption: dynamodb.TableEncryption.AWS_MANAGED,
      timeToLiveAttribute: 'ttl',
      removalPolicy,
    });

    // Enable auto-scaling for silo tier (higher capacity for requests table)
    if (props.tier === 'silo') {
      const readScaling = this.requestsTable.autoScaleReadCapacity({
        minCapacity: 50,
        maxCapacity: 2000,
      });
      readScaling.scaleOnUtilization({
        targetUtilizationPercent: 70,
      });

      const writeScaling = this.requestsTable.autoScaleWriteCapacity({
        minCapacity: 50,
        maxCapacity: 2000,
      });
      writeScaling.scaleOnUtilization({
        targetUtilizationPercent: 70,
      });
    }

    // GSI: Look up pending requests by user
    this.requestsTable.addGlobalSecondaryIndex({
      indexName: 'UserRequestsIndex',
      partitionKey: {
        name: 'user_id',
        type: dynamodb.AttributeType.STRING
      },
      sortKey: {
        name: 'created_at',
        type: dynamodb.AttributeType.STRING
      },
      projectionType: dynamodb.ProjectionType.ALL,
    });

    // Outputs
    new cdk.CfnOutput(this, 'ContractsTableName', {
      value: this.contractsTable.tableName,
      description: 'Contracts DynamoDB table name',
      exportName: 'VettID-ContractsTableName',
    });

    new cdk.CfnOutput(this, 'RequestsTableName', {
      value: this.requestsTable.tableName,
      description: 'Requests DynamoDB table name',
      exportName: 'VettID-RequestsTableName',
    });
  }
}
