import * as cdk from 'aws-cdk-lib';
import * as kms from 'aws-cdk-lib/aws-kms';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as secretsmanager from 'aws-cdk-lib/aws-secretsmanager';
import { Construct } from 'constructs';

export interface SecurityStackProps extends cdk.StackProps {
  readonly tier: 'pool' | 'silo';
}

/**
 * SecurityStack creates KMS keys, IAM roles, and manages secrets
 * for the VettID Service Vault.
 *
 * Multi-tenant isolation is enforced through:
 * - KMS encryption context including service_id
 * - Secrets Manager namespacing under /services/{service_id}/
 * - IAM policies that explicitly deny dangerous operations
 */
export class SecurityStack extends cdk.Stack {
  public readonly encryptionKey: kms.Key;
  public readonly vaultTaskRole: iam.Role;

  constructor(scope: Construct, id: string, props: SecurityStackProps) {
    super(scope, id, props);

    // KMS key for encrypting service vault data
    this.encryptionKey = new kms.Key(this, 'ServiceVaultKey', {
      alias: 'alias/vettid-service-vault',
      description: 'Encrypts VettID Service Vault data',
      enableKeyRotation: true,
      removalPolicy: props.tier === 'silo'
        ? cdk.RemovalPolicy.RETAIN
        : cdk.RemovalPolicy.DESTROY,
      // Key policy allows DynamoDB to use the key
      policy: new iam.PolicyDocument({
        statements: [
          // Allow account root full access (required)
          new iam.PolicyStatement({
            sid: 'AllowRootAccess',
            effect: iam.Effect.ALLOW,
            principals: [new iam.AccountRootPrincipal()],
            actions: ['kms:*'],
            resources: ['*'],
          }),
          // Allow DynamoDB to use the key
          new iam.PolicyStatement({
            sid: 'AllowDynamoDBAccess',
            effect: iam.Effect.ALLOW,
            principals: [new iam.ServicePrincipal('dynamodb.amazonaws.com')],
            actions: [
              'kms:Encrypt',
              'kms:Decrypt',
              'kms:ReEncrypt*',
              'kms:GenerateDataKey*',
              'kms:DescribeKey',
            ],
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

    // ECS Task Role (for application-level permissions)
    // Note: Execution role is created in ComputeStack to avoid cross-stack
    // circular dependencies with CloudWatch Logs permissions.
    this.vaultTaskRole = new iam.Role(this, 'VaultTaskRole', {
      roleName: 'vettid-service-vault-task',
      assumedBy: new iam.ServicePrincipal('ecs-tasks.amazonaws.com'),
    });

    // CRITICAL: Deny Scan operations to prevent cross-tenant data leakage
    this.vaultTaskRole.addToPolicy(new iam.PolicyStatement({
      sid: 'DenyScanOperations',
      effect: iam.Effect.DENY,
      actions: ['dynamodb:Scan'],
      resources: ['*'],
    }));

    // Deny dangerous IAM and STS operations
    this.vaultTaskRole.addToPolicy(new iam.PolicyStatement({
      sid: 'DenyDangerousOperations',
      effect: iam.Effect.DENY,
      actions: [
        'iam:*',
        'sts:AssumeRole',
        'ec2:Describe*',
        'ecs:Describe*',
      ],
      resources: ['*'],
    }));

    // Allow reading service-specific secrets
    this.vaultTaskRole.addToPolicy(new iam.PolicyStatement({
      sid: 'AllowServiceSecrets',
      effect: iam.Effect.ALLOW,
      actions: ['secretsmanager:GetSecretValue'],
      resources: [
        `arn:aws:secretsmanager:${this.region}:${this.account}:secret:/services/*`,
      ],
    }));

    // Allow using KMS key for encryption
    this.vaultTaskRole.addToPolicy(new iam.PolicyStatement({
      sid: 'AllowKMSEncryption',
      effect: iam.Effect.ALLOW,
      actions: [
        'kms:Encrypt',
        'kms:Decrypt',
        'kms:GenerateDataKey',
      ],
      resources: [this.encryptionKey.keyArn],
    }));

    // Allow CloudWatch logging
    this.vaultTaskRole.addToPolicy(new iam.PolicyStatement({
      sid: 'AllowLogging',
      effect: iam.Effect.ALLOW,
      actions: [
        'logs:CreateLogStream',
        'logs:PutLogEvents',
      ],
      resources: [
        `arn:aws:logs:${this.region}:${this.account}:log-group:/vettid/service-vault/*`,
      ],
    }));

    // Vault configuration secret
    new secretsmanager.Secret(this, 'VaultConfigSecret', {
      secretName: '/vettid/vault/config',
      description: 'VettID Service Vault configuration',
      generateSecretString: {
        secretStringTemplate: JSON.stringify({
          tier: props.tier,
        }),
        generateStringKey: 'api_key',
      },
    });

    // Outputs
    new cdk.CfnOutput(this, 'EncryptionKeyArn', {
      value: this.encryptionKey.keyArn,
      description: 'KMS key ARN for encryption',
      exportName: 'VettID-EncryptionKeyArn',
    });

    new cdk.CfnOutput(this, 'TaskRoleArn', {
      value: this.vaultTaskRole.roleArn,
      description: 'ECS Task Role ARN',
      exportName: 'VettID-TaskRoleArn',
    });
  }

  /**
   * Grant DynamoDB permissions to the task role.
   * Called by DatabaseStack after tables are created.
   */
  public grantDynamoDbAccess(tableArns: string[]): void {
    this.vaultTaskRole.addToPolicy(new iam.PolicyStatement({
      sid: 'AllowDynamoDBAccess',
      effect: iam.Effect.ALLOW,
      actions: [
        'dynamodb:GetItem',
        'dynamodb:PutItem',
        'dynamodb:UpdateItem',
        'dynamodb:DeleteItem',
        'dynamodb:Query',
        'dynamodb:BatchGetItem',
        'dynamodb:BatchWriteItem',
      ],
      resources: [
        ...tableArns,
        ...tableArns.map(arn => `${arn}/index/*`),
      ],
    }));
  }
}
