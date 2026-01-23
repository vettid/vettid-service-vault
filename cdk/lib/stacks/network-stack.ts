import * as cdk from 'aws-cdk-lib';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import { Construct } from 'constructs';

export interface NetworkStackProps extends cdk.StackProps {}

/**
 * NetworkStack creates the VPC and networking infrastructure
 * for the VettID Service Vault.
 */
export class NetworkStack extends cdk.Stack {
  public readonly vpc: ec2.Vpc;
  public readonly vaultSecurityGroup: ec2.SecurityGroup;

  constructor(scope: Construct, id: string, props?: NetworkStackProps) {
    super(scope, id, props);

    // Create VPC with public and private subnets
    this.vpc = new ec2.Vpc(this, 'VettIDVpc', {
      maxAzs: 2,
      natGateways: 1,
      subnetConfiguration: [
        {
          name: 'public',
          subnetType: ec2.SubnetType.PUBLIC,
          cidrMask: 24,
        },
        {
          name: 'private',
          subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS,
          cidrMask: 24,
        },
      ],
    });

    // Security group for vault services
    this.vaultSecurityGroup = new ec2.SecurityGroup(this, 'VaultSecurityGroup', {
      vpc: this.vpc,
      description: 'Security group for VettID Service Vault',
      allowAllOutbound: true,
    });

    // Allow inbound HTTPS from anywhere (API Gateway will front this)
    this.vaultSecurityGroup.addIngressRule(
      ec2.Peer.anyIpv4(),
      ec2.Port.tcp(443),
      'Allow HTTPS inbound'
    );

    // Allow internal communication
    this.vaultSecurityGroup.addIngressRule(
      this.vaultSecurityGroup,
      ec2.Port.allTraffic(),
      'Allow internal traffic'
    );

    // Outputs
    new cdk.CfnOutput(this, 'VpcId', {
      value: this.vpc.vpcId,
      description: 'VPC ID',
      exportName: 'VettID-VpcId',
    });
  }
}
