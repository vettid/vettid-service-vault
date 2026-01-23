import { APIGatewayProxyEvent, APIGatewayProxyResult } from 'aws-lambda';
import { DynamoDBClient, PutItemCommand, GetItemCommand } from '@aws-sdk/client-dynamodb';
import { marshall, unmarshall } from '@aws-sdk/util-dynamodb';
import { createHash, verify } from 'crypto';

const dynamodb = new DynamoDBClient({});
const tableName = process.env.REGISTRY_TABLE_NAME!;

interface RegistrationRequest {
  service_id: string;
  public_key: string;          // Ed25519 public key (base64)
  encryption_key: string;       // X25519 public key (base64)
  service_name: string;
  service_type: string;
  nats_endpoint: string;
  domain?: string;
  offerings: ServiceOffering[];
  signature: string;            // Signature of registration data
}

interface ServiceOffering {
  offering_id: string;
  name: string;
  description: string;
  capabilities: string[];
  required_data: DataRequirement[];
  pricing?: Pricing;
  terms_url?: string;
  terms_hash?: string;
}

interface DataRequirement {
  data_type: string;
  required: boolean;
  purpose: string;
}

interface Pricing {
  type: 'free' | 'one_time' | 'subscription';
  amount?: number;
  currency?: string;
  interval?: string;
}

interface ServiceRecord {
  service_id: string;
  public_key: string;
  encryption_key: string;
  service_name: string;
  service_type: string;
  nats_endpoint: string;
  domain?: string;
  domain_verified: boolean;
  offerings: ServiceOffering[];
  registered_at: string;
  updated_at: string;
  attestations: Attestation[];
}

interface Attestation {
  type: string;
  issuer: string;
  issued_at: string;
  expires_at: string;
  signature: string;
}

/**
 * Derive service_id from public key.
 * service_id = base58(sha256(public_key)[0:20])
 */
function deriveServiceId(publicKeyBase64: string): string {
  const publicKey = Buffer.from(publicKeyBase64, 'base64');
  const hash = createHash('sha256').update(publicKey).digest();
  const truncated = hash.subarray(0, 20);
  return encodeBase58(truncated);
}

/**
 * Simple Base58 encoding (Bitcoin alphabet).
 */
function encodeBase58(buffer: Buffer): string {
  const ALPHABET = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';
  let num = BigInt('0x' + buffer.toString('hex'));
  let result = '';

  while (num > 0n) {
    const mod = Number(num % 58n);
    result = ALPHABET[mod] + result;
    num = num / 58n;
  }

  // Handle leading zeros
  for (const byte of buffer) {
    if (byte === 0) {
      result = ALPHABET[0] + result;
    } else {
      break;
    }
  }

  return result || ALPHABET[0];
}

export async function handler(event: APIGatewayProxyEvent): Promise<APIGatewayProxyResult> {
  try {
    if (!event.body) {
      return errorResponse(400, 'Request body is required');
    }

    const request: RegistrationRequest = JSON.parse(event.body);

    // Validate required fields
    if (!request.public_key || !request.service_name || !request.service_type) {
      return errorResponse(400, 'Missing required fields: public_key, service_name, service_type');
    }

    // Derive and validate service_id
    const derivedServiceId = deriveServiceId(request.public_key);
    if (request.service_id && request.service_id !== derivedServiceId) {
      return errorResponse(400, 'service_id does not match public_key derivation');
    }

    // Check if service already exists
    const existing = await dynamodb.send(new GetItemCommand({
      TableName: tableName,
      Key: marshall({ service_id: derivedServiceId }),
    }));

    if (existing.Item) {
      return errorResponse(409, 'Service already registered');
    }

    // TODO: Verify domain ownership via DNS TXT record
    // For now, domain_verified is always false until manual verification
    const domainVerified = false;

    // Create service record
    const now = new Date().toISOString();
    const serviceRecord: ServiceRecord = {
      service_id: derivedServiceId,
      public_key: request.public_key,
      encryption_key: request.encryption_key,
      service_name: request.service_name,
      service_type: request.service_type,
      nats_endpoint: request.nats_endpoint,
      domain: request.domain,
      domain_verified: domainVerified,
      offerings: request.offerings || [],
      registered_at: now,
      updated_at: now,
      attestations: [],
    };

    // Store in DynamoDB
    await dynamodb.send(new PutItemCommand({
      TableName: tableName,
      Item: marshall(serviceRecord, { removeUndefinedValues: true }),
      ConditionExpression: 'attribute_not_exists(service_id)',
    }));

    // Return success with the registered service info
    return {
      statusCode: 201,
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        service_id: derivedServiceId,
        service_name: request.service_name,
        domain_verified: domainVerified,
        registered_at: now,
        message: 'Service registered successfully',
      }),
    };
  } catch (error) {
    console.error('Registration error:', error);

    if ((error as any).name === 'ConditionalCheckFailedException') {
      return errorResponse(409, 'Service already registered');
    }

    return errorResponse(500, 'Internal server error');
  }
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
