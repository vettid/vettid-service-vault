import * as cdk from 'aws-cdk-lib';
import * as cloudwatch from 'aws-cdk-lib/aws-cloudwatch';
import * as cloudwatchActions from 'aws-cdk-lib/aws-cloudwatch-actions';
import * as sns from 'aws-cdk-lib/aws-sns';
import * as ecs from 'aws-cdk-lib/aws-ecs';
import * as elbv2 from 'aws-cdk-lib/aws-elasticloadbalancingv2';
import * as dynamodb from 'aws-cdk-lib/aws-dynamodb';
import { Construct } from 'constructs';

export interface MonitoringStackProps extends cdk.StackProps {
  readonly cluster: ecs.ICluster;
  readonly service: ecs.IBaseService;
  readonly loadBalancer: elbv2.ApplicationLoadBalancer;
  readonly contractsTable: dynamodb.ITable;
  readonly requestsTable: dynamodb.ITable;
  readonly tier: 'pool' | 'silo';
}

/**
 * MonitoringStack creates CloudWatch dashboards and alarms
 * for the VettID Service Vault.
 */
export class MonitoringStack extends cdk.Stack {
  public readonly dashboard: cloudwatch.Dashboard;
  public readonly alarmTopic: sns.Topic;

  constructor(scope: Construct, id: string, props: MonitoringStackProps) {
    super(scope, id, props);

    // SNS Topic for alarms
    this.alarmTopic = new sns.Topic(this, 'AlarmTopic', {
      topicName: 'vettid-service-vault-alarms',
      displayName: 'VettID Service Vault Alarms',
    });

    // Metrics
    const albTargetGroup = `targetgroup/${props.loadBalancer.loadBalancerName}`;

    // API Latency (p99)
    const latencyMetric = new cloudwatch.Metric({
      namespace: 'AWS/ApplicationELB',
      metricName: 'TargetResponseTime',
      dimensionsMap: {
        LoadBalancer: props.loadBalancer.loadBalancerFullName,
      },
      statistic: 'p99',
      period: cdk.Duration.minutes(1),
    });

    // API Error Rate (5xx)
    const errorRateMetric = new cloudwatch.MathExpression({
      expression: '(errors / requests) * 100',
      usingMetrics: {
        errors: new cloudwatch.Metric({
          namespace: 'AWS/ApplicationELB',
          metricName: 'HTTPCode_Target_5XX_Count',
          dimensionsMap: {
            LoadBalancer: props.loadBalancer.loadBalancerFullName,
          },
          statistic: 'Sum',
          period: cdk.Duration.minutes(1),
        }),
        requests: new cloudwatch.Metric({
          namespace: 'AWS/ApplicationELB',
          metricName: 'RequestCount',
          dimensionsMap: {
            LoadBalancer: props.loadBalancer.loadBalancerFullName,
          },
          statistic: 'Sum',
          period: cdk.Duration.minutes(1),
        }),
      },
      period: cdk.Duration.minutes(1),
    });

    // ECS CPU Utilization
    const cpuMetric = new cloudwatch.Metric({
      namespace: 'AWS/ECS',
      metricName: 'CPUUtilization',
      dimensionsMap: {
        ClusterName: props.cluster.clusterName,
        ServiceName: 'service-vault',
      },
      statistic: 'Average',
      period: cdk.Duration.minutes(1),
    });

    // ECS Memory Utilization
    const memoryMetric = new cloudwatch.Metric({
      namespace: 'AWS/ECS',
      metricName: 'MemoryUtilization',
      dimensionsMap: {
        ClusterName: props.cluster.clusterName,
        ServiceName: 'service-vault',
      },
      statistic: 'Average',
      period: cdk.Duration.minutes(1),
    });

    // DynamoDB Throttled Requests
    const contractsThrottleMetric = props.contractsTable.metricThrottledRequests({
      period: cdk.Duration.minutes(1),
    });

    const requestsThrottleMetric = props.requestsTable.metricThrottledRequests({
      period: cdk.Duration.minutes(1),
    });

    // Alarms
    // High Latency Alarm
    const latencyAlarm = new cloudwatch.Alarm(this, 'HighLatencyAlarm', {
      alarmName: 'vettid-service-vault-high-latency',
      alarmDescription: 'API latency (p99) exceeds 1 second',
      metric: latencyMetric,
      threshold: 1, // 1 second
      evaluationPeriods: 3,
      comparisonOperator: cloudwatch.ComparisonOperator.GREATER_THAN_THRESHOLD,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });

    // High Error Rate Alarm
    const errorAlarm = new cloudwatch.Alarm(this, 'HighErrorRateAlarm', {
      alarmName: 'vettid-service-vault-high-error-rate',
      alarmDescription: 'API error rate exceeds 1%',
      metric: errorRateMetric,
      threshold: 1, // 1%
      evaluationPeriods: 3,
      comparisonOperator: cloudwatch.ComparisonOperator.GREATER_THAN_THRESHOLD,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });

    // High CPU Alarm
    const cpuAlarm = new cloudwatch.Alarm(this, 'HighCpuAlarm', {
      alarmName: 'vettid-service-vault-high-cpu',
      alarmDescription: 'ECS CPU utilization exceeds 85%',
      metric: cpuMetric,
      threshold: 85,
      evaluationPeriods: 3,
      comparisonOperator: cloudwatch.ComparisonOperator.GREATER_THAN_THRESHOLD,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });

    // DynamoDB Throttle Alarm
    const throttleAlarm = new cloudwatch.Alarm(this, 'DynamoDbThrottleAlarm', {
      alarmName: 'vettid-service-vault-dynamodb-throttle',
      alarmDescription: 'DynamoDB requests are being throttled',
      metric: new cloudwatch.MathExpression({
        expression: 'contracts + requests',
        usingMetrics: {
          contracts: contractsThrottleMetric,
          requests: requestsThrottleMetric,
        },
      }),
      threshold: 1,
      evaluationPeriods: 1,
      comparisonOperator: cloudwatch.ComparisonOperator.GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
      treatMissingData: cloudwatch.TreatMissingData.NOT_BREACHING,
    });

    // Add alarm actions
    const alarmAction = new cloudwatchActions.SnsAction(this.alarmTopic);
    latencyAlarm.addAlarmAction(alarmAction);
    errorAlarm.addAlarmAction(alarmAction);
    cpuAlarm.addAlarmAction(alarmAction);
    throttleAlarm.addAlarmAction(alarmAction);

    // Dashboard
    this.dashboard = new cloudwatch.Dashboard(this, 'ServiceVaultDashboard', {
      dashboardName: 'vettid-service-vault',
    });

    // Row 1: API Performance
    this.dashboard.addWidgets(
      new cloudwatch.GraphWidget({
        title: 'API Latency (p99)',
        left: [latencyMetric],
        width: 12,
        height: 6,
      }),
      new cloudwatch.GraphWidget({
        title: 'API Error Rate (%)',
        left: [errorRateMetric],
        width: 12,
        height: 6,
        leftYAxis: {
          min: 0,
          max: 10,
        },
      }),
    );

    // Row 2: Compute Resources
    this.dashboard.addWidgets(
      new cloudwatch.GraphWidget({
        title: 'ECS CPU Utilization',
        left: [cpuMetric],
        width: 12,
        height: 6,
        leftAnnotations: [
          { value: 85, color: '#ff7f0e', label: 'Alarm threshold' },
        ],
      }),
      new cloudwatch.GraphWidget({
        title: 'ECS Memory Utilization',
        left: [memoryMetric],
        width: 12,
        height: 6,
        leftAnnotations: [
          { value: 80, color: '#ff7f0e', label: 'Scale threshold' },
        ],
      }),
    );

    // Row 3: DynamoDB
    this.dashboard.addWidgets(
      new cloudwatch.GraphWidget({
        title: 'DynamoDB Read Capacity',
        left: [
          props.contractsTable.metricConsumedReadCapacityUnits(),
          props.requestsTable.metricConsumedReadCapacityUnits(),
        ],
        width: 12,
        height: 6,
      }),
      new cloudwatch.GraphWidget({
        title: 'DynamoDB Write Capacity',
        left: [
          props.contractsTable.metricConsumedWriteCapacityUnits(),
          props.requestsTable.metricConsumedWriteCapacityUnits(),
        ],
        width: 12,
        height: 6,
      }),
    );

    // Row 4: Request Counts and Throttles
    this.dashboard.addWidgets(
      new cloudwatch.GraphWidget({
        title: 'Request Count',
        left: [
          new cloudwatch.Metric({
            namespace: 'AWS/ApplicationELB',
            metricName: 'RequestCount',
            dimensionsMap: {
              LoadBalancer: props.loadBalancer.loadBalancerFullName,
            },
            statistic: 'Sum',
            period: cdk.Duration.minutes(1),
          }),
        ],
        width: 12,
        height: 6,
      }),
      new cloudwatch.GraphWidget({
        title: 'DynamoDB Throttled Requests',
        left: [contractsThrottleMetric, requestsThrottleMetric],
        width: 12,
        height: 6,
      }),
    );

    // Row 5: Alarm Status
    this.dashboard.addWidgets(
      new cloudwatch.AlarmStatusWidget({
        title: 'Alarm Status',
        alarms: [latencyAlarm, errorAlarm, cpuAlarm, throttleAlarm],
        width: 24,
        height: 3,
      }),
    );

    // Outputs
    new cdk.CfnOutput(this, 'DashboardUrl', {
      value: `https://${this.region}.console.aws.amazon.com/cloudwatch/home?region=${this.region}#dashboards:name=${this.dashboard.dashboardName}`,
      description: 'CloudWatch Dashboard URL',
      exportName: 'VettID-DashboardUrl',
    });

    new cdk.CfnOutput(this, 'AlarmTopicArn', {
      value: this.alarmTopic.topicArn,
      description: 'SNS Topic ARN for alarms',
      exportName: 'VettID-AlarmTopicArn',
    });
  }
}
