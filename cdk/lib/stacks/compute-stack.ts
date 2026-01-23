import * as cdk from 'aws-cdk-lib';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as ecs from 'aws-cdk-lib/aws-ecs';
import * as iam from 'aws-cdk-lib/aws-iam';
import * as elbv2 from 'aws-cdk-lib/aws-elasticloadbalancingv2';
import { Construct } from 'constructs';

export interface ComputeStackProps extends cdk.StackProps {
  readonly vpc: ec2.IVpc;
  readonly securityGroup: ec2.ISecurityGroup;
  readonly taskRole: iam.IRole;
  readonly tier: 'pool' | 'silo';
  readonly contractsTableName: string;
  readonly requestsTableName: string;
}

/**
 * ComputeStack creates ECS/Fargate infrastructure for the Service Vault.
 *
 * Pool tier uses:
 * - 60/40 Spot/On-Demand ratio for cost optimization
 * - ARM64 (Graviton) for additional savings
 * - Auto-scaling based on CPU and memory
 */
export class ComputeStack extends cdk.Stack {
  public readonly cluster: ecs.Cluster;
  public readonly service: ecs.FargateService;
  public readonly loadBalancer: elbv2.ApplicationLoadBalancer;
  public readonly listener: elbv2.ApplicationListener;

  constructor(scope: Construct, id: string, props: ComputeStackProps) {
    super(scope, id, props);

    // ECS Cluster with Container Insights
    this.cluster = new ecs.Cluster(this, 'ServiceVaultCluster', {
      clusterName: 'vettid-service-vault',
      vpc: props.vpc,
      containerInsights: true,
      enableFargateCapacityProviders: true,
    });

    // ECS Task Execution Role - created in this stack to avoid cross-stack
    // circular dependencies with CloudWatch Logs permissions
    const executionRole = new iam.Role(this, 'ExecutionRole', {
      roleName: 'vettid-service-vault-execution',
      assumedBy: new iam.ServicePrincipal('ecs-tasks.amazonaws.com'),
      managedPolicies: [
        iam.ManagedPolicy.fromAwsManagedPolicyName(
          'service-role/AmazonECSTaskExecutionRolePolicy'
        ),
      ],
    });

    // Task Definition
    const taskDefinition = new ecs.FargateTaskDefinition(this, 'ServiceVaultTask', {
      family: 'vettid-service-vault',
      memoryLimitMiB: 512,
      cpu: 256,
      taskRole: props.taskRole,
      executionRole,
      runtimePlatform: {
        cpuArchitecture: ecs.CpuArchitecture.ARM64,
        operatingSystemFamily: ecs.OperatingSystemFamily.LINUX,
      },
    });

    // Container Definition
    // Uses placeholder image; will be replaced with actual image in deployment
    const container = taskDefinition.addContainer('vault', {
      containerName: 'service-vault',
      image: ecs.ContainerImage.fromRegistry('public.ecr.aws/docker/library/alpine:latest'),
      // Use awsLogs with just a stream prefix - CDK will create the log group
      // and add permissions to the execution role automatically
      logging: ecs.LogDrivers.awsLogs({
        streamPrefix: 'service-vault',
      }),
      environment: {
        CONTRACTS_TABLE: props.contractsTableName,
        REQUESTS_TABLE: props.requestsTableName,
        AWS_REGION: this.region,
        TIER: props.tier,
      },
      healthCheck: {
        command: ['CMD-SHELL', 'wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1'],
        interval: cdk.Duration.seconds(30),
        timeout: cdk.Duration.seconds(5),
        retries: 3,
        startPeriod: cdk.Duration.seconds(60),
      },
      portMappings: [
        {
          containerPort: 8080,
          protocol: ecs.Protocol.TCP,
        },
      ],
    });

    // Application Load Balancer
    this.loadBalancer = new elbv2.ApplicationLoadBalancer(this, 'ServiceVaultAlb', {
      loadBalancerName: 'vettid-service-vault',
      vpc: props.vpc,
      internetFacing: true,
      securityGroup: props.securityGroup,
    });

    // HTTPS Listener (placeholder - requires certificate)
    // For now, create HTTP listener for development
    this.listener = this.loadBalancer.addListener('HttpListener', {
      port: 80,
      protocol: elbv2.ApplicationProtocol.HTTP,
    });

    // Fargate Service with capacity provider strategy
    this.service = new ecs.FargateService(this, 'ServiceVaultService', {
      serviceName: 'service-vault',
      cluster: this.cluster,
      taskDefinition,
      desiredCount: 2,
      minHealthyPercent: 50,
      maxHealthyPercent: 200,
      assignPublicIp: false,
      vpcSubnets: {
        subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS,
      },
      securityGroups: [props.securityGroup],
      // Extended stop timeout for graceful shutdown (drain NATS connections)
      enableExecuteCommand: true,
      // Capacity provider strategy: 60% Spot, 40% On-Demand
      capacityProviderStrategies: [
        {
          capacityProvider: 'FARGATE_SPOT',
          weight: 60,
        },
        {
          capacityProvider: 'FARGATE',
          weight: 40,
          base: 2, // Always keep 2 on-demand tasks running
        },
      ],
      circuitBreaker: {
        rollback: true,
      },
    });

    // Register with ALB
    const targetGroup = this.listener.addTargets('ServiceVaultTarget', {
      targetGroupName: 'vettid-service-vault',
      port: 8080,
      protocol: elbv2.ApplicationProtocol.HTTP,
      targets: [this.service],
      healthCheck: {
        path: '/health',
        interval: cdk.Duration.seconds(30),
        timeout: cdk.Duration.seconds(5),
        healthyThresholdCount: 2,
        unhealthyThresholdCount: 3,
      },
      deregistrationDelay: cdk.Duration.seconds(30),
    });

    // Auto-scaling
    const scaling = this.service.autoScaleTaskCount({
      minCapacity: 2,
      maxCapacity: props.tier === 'silo' ? 50 : 20,
    });

    // Scale on CPU utilization
    scaling.scaleOnCpuUtilization('CpuScaling', {
      targetUtilizationPercent: 70,
      scaleInCooldown: cdk.Duration.seconds(60),
      scaleOutCooldown: cdk.Duration.seconds(60),
    });

    // Scale on memory utilization
    scaling.scaleOnMemoryUtilization('MemoryScaling', {
      targetUtilizationPercent: 80,
      scaleInCooldown: cdk.Duration.seconds(60),
      scaleOutCooldown: cdk.Duration.seconds(60),
    });

    // Scale on request count
    scaling.scaleOnRequestCount('RequestScaling', {
      requestsPerTarget: 1000,
      targetGroup,
      scaleInCooldown: cdk.Duration.seconds(60),
      scaleOutCooldown: cdk.Duration.seconds(60),
    });

    // Outputs
    new cdk.CfnOutput(this, 'ClusterArn', {
      value: this.cluster.clusterArn,
      description: 'ECS Cluster ARN',
      exportName: 'VettID-ClusterArn',
    });

    new cdk.CfnOutput(this, 'ServiceArn', {
      value: this.service.serviceArn,
      description: 'ECS Service ARN',
      exportName: 'VettID-ServiceArn',
    });

    new cdk.CfnOutput(this, 'LoadBalancerDns', {
      value: this.loadBalancer.loadBalancerDnsName,
      description: 'ALB DNS name',
      exportName: 'VettID-AlbDns',
    });
  }
}
