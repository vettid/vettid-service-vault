import { APIGatewayProxyEvent, APIGatewayProxyResult } from 'aws-lambda';
import { DynamoDBClient, GetItemCommand, ScanCommand, QueryCommand } from '@aws-sdk/client-dynamodb';
import { marshall, unmarshall } from '@aws-sdk/util-dynamodb';

const dynamodb = new DynamoDBClient({});
const tableName = process.env.REGISTRY_TABLE_NAME!;

interface ServiceListItem {
  service_id: string;
  service_name: string;
  service_type: string;
  domain?: string;
  domain_verified: boolean;
  offering_count: number;
  has_attestations: boolean;
}

interface ServiceDetail {
  service_id: string;
  public_key: string;
  encryption_key: string;
  service_name: string;
  service_type: string;
  nats_endpoint: string;
  domain?: string;
  domain_verified: boolean;
  offerings: any[];
  attestations: any[];
  registered_at: string;
}

export async function handler(event: APIGatewayProxyEvent): Promise<APIGatewayProxyResult> {
  try {
    const serviceId = event.pathParameters?.serviceId;

    if (serviceId) {
      // GET /public/services/{serviceId}
      return await getServiceById(serviceId);
    } else {
      // GET /public/services
      return await listServices(event);
    }
  } catch (error) {
    console.error('Directory error:', error);
    return errorResponse(500, 'Internal server error');
  }
}

async function getServiceById(serviceId: string): Promise<APIGatewayProxyResult> {
  const result = await dynamodb.send(new GetItemCommand({
    TableName: tableName,
    Key: marshall({ service_id: serviceId }),
  }));

  if (!result.Item) {
    return errorResponse(404, 'Service not found');
  }

  const record = unmarshall(result.Item);

  // Return public service details for connection
  const serviceDetail: ServiceDetail = {
    service_id: record.service_id,
    public_key: record.public_key,
    encryption_key: record.encryption_key,
    service_name: record.service_name,
    service_type: record.service_type,
    nats_endpoint: record.nats_endpoint,
    domain: record.domain,
    domain_verified: record.domain_verified,
    offerings: record.offerings || [],
    attestations: record.attestations || [],
    registered_at: record.registered_at,
  };

  return {
    statusCode: 200,
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(serviceDetail),
  };
}

async function listServices(event: APIGatewayProxyEvent): Promise<APIGatewayProxyResult> {
  const queryParams = event.queryStringParameters || {};
  const limit = Math.min(parseInt(queryParams.limit || '20', 10), 100);
  const serviceType = queryParams.type;

  let items: any[];

  if (serviceType) {
    // Query by service type using GSI
    const result = await dynamodb.send(new QueryCommand({
      TableName: tableName,
      IndexName: 'TypeIndex',
      KeyConditionExpression: 'service_type = :type',
      ExpressionAttributeValues: marshall({
        ':type': serviceType,
      }),
      Limit: limit,
    }));
    items = result.Items || [];
  } else {
    // Scan all services (with pagination)
    const result = await dynamodb.send(new ScanCommand({
      TableName: tableName,
      Limit: limit,
      ExclusiveStartKey: queryParams.cursor
        ? marshall({ service_id: queryParams.cursor })
        : undefined,
    }));
    items = result.Items || [];
  }

  // Transform to list format
  const services: ServiceListItem[] = items.map(item => {
    const record = unmarshall(item);
    return {
      service_id: record.service_id,
      service_name: record.service_name,
      service_type: record.service_type,
      domain: record.domain,
      domain_verified: record.domain_verified,
      offering_count: (record.offerings || []).length,
      has_attestations: (record.attestations || []).length > 0,
    };
  });

  // Build response with pagination
  const response: any = {
    services,
    count: services.length,
  };

  // Include cursor for pagination if there are more results
  if (services.length === limit) {
    response.next_cursor = services[services.length - 1]?.service_id;
  }

  return {
    statusCode: 200,
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(response),
  };
}

function errorResponse(statusCode: number, message: string): APIGatewayProxyResult {
  return {
    statusCode,
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      error: message,
    }),
  };
}
