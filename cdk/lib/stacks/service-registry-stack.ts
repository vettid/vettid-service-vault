import * as cdk from 'aws-cdk-lib';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import * as lambda from 'aws-cdk-lib/aws-lambda';
import * as lambdaNode from 'aws-cdk-lib/aws-lambda-nodejs';
import * as apigateway from 'aws-cdk-lib/aws-apigateway';
import * as logs from 'aws-cdk-lib/aws-logs';
import { Construct } from 'constructs';
import * as path from 'path';

export interface ServiceRegistryStackProps extends cdk.StackProps {}

/**
 * ServiceRegistryStack creates the VettID service registry infrastructure.
 *
 * The registry allows:
 * - Services to register their identity and capabilities
 * - VettID to issue attestations for verified services
 * - Users to discover available services
 */
export class ServiceRegistryStack extends cdk.Stack {
  public readonly registryTable: dynamodb.Table;
  public readonly registrationApi: apigateway.RestApi;

  constructor(scope: Construct, id: string, props?: ServiceRegistryStackProps) {
    super(scope, id, props);

    // DynamoDB table for registered services
    this.registryTable = new dynamodb.Table(this, 'ServiceRegistry', {
      tableName: 'vettid-service-registry',
      partitionKey: {
        name: 'service_id',
        type: dynamodb.AttributeType.STRING
      },
      billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
      encryption: dynamodb.TableEncryption.AWS_MANAGED,
      pointInTimeRecoverySpecification: { pointInTimeRecoveryEnabled: true },
      removalPolicy: cdk.RemovalPolicy.RETAIN,
    });

    // GSI for domain lookups (verified domains only)
    this.registryTable.addGlobalSecondaryIndex({
      indexName: 'DomainIndex',
      partitionKey: {
        name: 'domain',
        type: dynamodb.AttributeType.STRING
      },
      projectionType: dynamodb.ProjectionType.ALL,
    });

    // GSI for service type filtering
    this.registryTable.addGlobalSecondaryIndex({
      indexName: 'TypeIndex',
      partitionKey: {
        name: 'service_type',
        type: dynamodb.AttributeType.STRING
      },
      sortKey: {
        name: 'service_name',
        type: dynamodb.AttributeType.STRING
      },
      projectionType: dynamodb.ProjectionType.ALL,
    });

    // Registration Lambda
    const registrationFunction = new lambdaNode.NodejsFunction(this, 'RegistrationFunction', {
      entry: path.join(__dirname, '../lambda/registration/index.ts'),
      handler: 'handler',
      runtime: lambda.Runtime.NODEJS_20_X,
      architecture: lambda.Architecture.ARM_64,
      memorySize: 256,
      timeout: cdk.Duration.seconds(30),
      environment: {
        REGISTRY_TABLE_NAME: this.registryTable.tableName,
      },
      logRetention: logs.RetentionDays.ONE_MONTH,
    });

    // Grant Lambda access to DynamoDB
    this.registryTable.grantReadWriteData(registrationFunction);

    // Directory Lambda
    const directoryFunction = new lambdaNode.NodejsFunction(this, 'DirectoryFunction', {
      entry: path.join(__dirname, '../lambda/directory/index.ts'),
      handler: 'handler',
      runtime: lambda.Runtime.NODEJS_20_X,
      architecture: lambda.Architecture.ARM_64,
      memorySize: 256,
      timeout: cdk.Duration.seconds(10),
      environment: {
        REGISTRY_TABLE_NAME: this.registryTable.tableName,
      },
      logRetention: logs.RetentionDays.ONE_MONTH,
    });

    // Grant Lambda read access to DynamoDB
    this.registryTable.grantReadData(directoryFunction);

    // API Gateway
    this.registrationApi = new apigateway.RestApi(this, 'ServiceRegistryApi', {
      restApiName: 'VettID Service Registry',
      description: 'API for VettID service registration and discovery',
      deployOptions: {
        stageName: 'v1',
        throttlingRateLimit: 100,
        throttlingBurstLimit: 200,
      },
      defaultCorsPreflightOptions: {
        allowOrigins: apigateway.Cors.ALL_ORIGINS,
        allowMethods: ['GET', 'POST', 'OPTIONS'],
      },
    });

    // Admin endpoints (requires authentication)
    const adminResource = this.registrationApi.root.addResource('admin');
    const servicesAdminResource = adminResource.addResource('services');

    // POST /admin/services/register
    const registerResource = servicesAdminResource.addResource('register');
    registerResource.addMethod('POST', new apigateway.LambdaIntegration(registrationFunction), {
      apiKeyRequired: true, // Require API key for admin endpoints
    });

    // Public endpoints
    const publicResource = this.registrationApi.root.addResource('public');
    const servicesPublicResource = publicResource.addResource('services');

    // GET /public/services
    servicesPublicResource.addMethod('GET', new apigateway.LambdaIntegration(directoryFunction));

    // GET /public/services/{serviceId}
    const serviceByIdResource = servicesPublicResource.addResource('{serviceId}');
    serviceByIdResource.addMethod('GET', new apigateway.LambdaIntegration(directoryFunction));

    // API Key for admin access
    const apiKey = this.registrationApi.addApiKey('AdminApiKey', {
      apiKeyName: 'vettid-registry-admin-key',
    });

    const usagePlan = this.registrationApi.addUsagePlan('AdminUsagePlan', {
      name: 'Admin',
      throttle: {
        rateLimit: 100,
        burstLimit: 200,
      },
    });

    usagePlan.addApiKey(apiKey);
    usagePlan.addApiStage({
      stage: this.registrationApi.deploymentStage,
    });

    // Outputs
    new cdk.CfnOutput(this, 'RegistryTableName', {
      value: this.registryTable.tableName,
      description: 'Service Registry DynamoDB table name',
      exportName: 'VettID-RegistryTableName',
    });

    new cdk.CfnOutput(this, 'ApiEndpoint', {
      value: this.registrationApi.url,
      description: 'Service Registry API endpoint',
      exportName: 'VettID-RegistryApiEndpoint',
    });
  }
}
