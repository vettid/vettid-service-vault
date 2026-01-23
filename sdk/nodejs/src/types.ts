/**
 * VettID Service Vault SDK Types
 */

// ============================================================================
// Service Identity
// ============================================================================

/**
 * Service type categories
 */
export type ServiceType =
  | 'generic'
  | 'payment'
  | 'identity'
  | 'commerce'
  | 'healthcare'
  | 'finance';

/**
 * Service identity containing cryptographic keys and metadata
 */
export interface ServiceIdentity {
  serviceId: string;
  serviceName: string;
  serviceType: ServiceType;
  signingPublicKey: string; // Base64 Ed25519 public key
  encryptionPublicKey: string; // Base64 X25519 public key
  domain?: string;
  domainVerified?: boolean;
  natsEndpoint?: string;
}

/**
 * Service keypair for signing and encryption
 */
export interface ServiceKeyPair {
  signingPrivateKey: Uint8Array; // Ed25519 private key
  signingPublicKey: Uint8Array; // Ed25519 public key
  encryptionPrivateKey: Uint8Array; // X25519 private key
  encryptionPublicKey: Uint8Array; // X25519 public key
}

// ============================================================================
// Contracts
// ============================================================================

/**
 * Contract status
 */
export type ContractStatus =
  | 'pending'
  | 'active'
  | 'paused'
  | 'cancelled'
  | 'expired';

/**
 * Capability types that can be granted to a service
 */
export type CapabilityType =
  | 'authenticate'
  | 'authorize'
  | 'read_data'
  | 'write_data'
  | 'sign'
  | 'browse_data'
  | 'request_data'
  | 'notify'
  | 'call'
  | 'payment'
  | 'secrets';

/**
 * A specific capability granted to a service
 */
export interface CapabilityGrant {
  capability: CapabilityType;
  scope?: string;
  constraints?: Record<string, unknown>;
  expiresAt?: Date;
}

/**
 * Data requirement specification
 */
export interface DataRequirement {
  dataType: string;
  required: boolean;
  purpose: string;
  retention?: string;
  thirdParty?: boolean;
}

/**
 * Pricing information for an offering
 */
export interface Pricing {
  type: 'free' | 'one_time' | 'subscription';
  amount?: number;
  currency?: string;
  interval?: string;
}

/**
 * A service contract offering
 */
export interface ContractOffering {
  offeringId: string;
  name: string;
  description: string;
  capabilities: CapabilityGrant[];
  requiredData?: DataRequirement[];
  pricing?: Pricing;
  termsUrl?: string;
  termsHash?: string;
}

/**
 * A signed connection contract between user and service
 */
export interface ConnectionContract {
  contractId: string;
  userId: string;
  serviceId: string;
  offeringId: string;
  offering: ContractOffering;
  status: ContractStatus;
  createdAt: Date;
  activatedAt?: Date;
  cancelledAt?: Date;
}

// ============================================================================
// Requests & Responses
// ============================================================================

/**
 * Request status
 */
export type RequestStatus =
  | 'pending'
  | 'approved'
  | 'denied'
  | 'expired'
  | 'offline_approved';

/**
 * Authentication request
 */
export interface AuthRequest {
  requestId: string;
  userId: string;
  purpose: string;
  context?: Record<string, unknown>;
  expiresAt: Date;
  offlineGrace?: number; // milliseconds
  callbackUrl?: string;
}

/**
 * Authentication response
 */
export interface AuthResponse {
  requestId: string;
  status: RequestStatus;
  userId: string;
  timestamp: Date;
  sessionKey?: string;
}

/**
 * Authorization request
 */
export interface AuthzRequest {
  requestId: string;
  userId: string;
  action: string;
  resource: string;
  context?: Record<string, unknown>;
  expiresAt: Date;
  offlineGrace?: number;
  callbackUrl?: string;
}

/**
 * Authorization response
 */
export interface AuthzResponse {
  requestId: string;
  status: RequestStatus;
  userId: string;
  action: string;
  resource: string;
  timestamp: Date;
  expiresAt?: Date;
}

/**
 * Data request types
 */
export type DataRequestType = 'browse_metadata' | 'request_data';

/**
 * Data request
 */
export interface DataRequest {
  requestId: string;
  userId: string;
  requestType: DataRequestType;
  dataTypes?: string[];
  dataPaths?: string[];
  purpose: string;
  context?: Record<string, unknown>;
  expiresAt: Date;
  offlineGrace?: number;
  callbackUrl?: string;
}

/**
 * Data response
 */
export interface DataResponse {
  requestId: string;
  status: RequestStatus;
  data?: Record<string, unknown>;
  metadata?: DataTypeMetadata[];
  timestamp: Date;
  constraints?: Record<string, unknown>;
}

/**
 * Metadata about available data types
 */
export interface DataTypeMetadata {
  dataType: string;
  name: string;
  description?: string;
  available: boolean;
}

// ============================================================================
// Notifications
// ============================================================================

/**
 * Notification to send to a user
 */
export interface Notification {
  title: string;
  body: string;
  category?: string;
  priority?: 'low' | 'normal' | 'high' | 'urgent';
  data?: Record<string, unknown>;
  actionUrl?: string;
  imageUrl?: string;
  expiresAt?: Date;
}

/**
 * Notification result
 */
export interface NotificationResult {
  notificationId: string;
  delivered: boolean;
  timestamp: Date;
}

// ============================================================================
// Calls
// ============================================================================

/**
 * Call type
 */
export type CallType = 'voice' | 'video';

/**
 * Call status
 */
export type CallStatus =
  | 'initiating'
  | 'ringing'
  | 'connecting'
  | 'connected'
  | 'ended'
  | 'failed'
  | 'rejected'
  | 'missed'
  | 'busy';

/**
 * ICE server configuration
 */
export interface ICEServer {
  urls: string[];
  username?: string;
  credential?: string;
}

/**
 * WebRTC session description
 */
export interface RTCSessionDescription {
  type: 'offer' | 'answer';
  sdp: string;
}

/**
 * Call request
 */
export interface CallRequest {
  userId: string;
  type: CallType;
  purpose?: string;
  context?: Record<string, unknown>;
  iceServers?: ICEServer[];
  offer?: RTCSessionDescription;
  expiresIn?: number; // seconds
  callbackUrl?: string;
}

/**
 * Call result
 */
export interface CallResult {
  callId: string;
  requestId: string;
  status: CallStatus;
  answer?: RTCSessionDescription;
  startedAt?: Date;
  connectedAt?: Date;
  endedAt?: Date;
  duration?: number; // seconds
}

// ============================================================================
// Payments
// ============================================================================

/**
 * Monetary amount
 */
export interface Money {
  amount: number; // In smallest currency unit (cents)
  currency: string; // ISO 4217
}

/**
 * Merchant information
 */
export interface MerchantInfo {
  merchantId?: string;
  merchantName?: string;
  merchantUrl?: string;
  merchantLogo?: string;
  category?: string;
}

/**
 * Payment line item
 */
export interface PaymentItem {
  itemId?: string;
  name: string;
  description?: string;
  quantity: number;
  unitPrice: Money;
  imageUrl?: string;
}

/**
 * Recurring payment info
 */
export interface RecurringInfo {
  interval: 'day' | 'week' | 'month' | 'year';
  intervalCount: number;
  trialDays?: number;
}

/**
 * Payment request
 */
export interface PaymentRequest {
  userId: string;
  amount: Money;
  description: string;
  merchantInfo?: MerchantInfo;
  items?: PaymentItem[];
  allowedMethods?: string[];
  recurringInfo?: RecurringInfo;
  callbackUrl?: string;
  expiresIn?: number; // seconds
  metadata?: Record<string, unknown>;
}

/**
 * Payment status
 */
export type PaymentStatus =
  | 'pending'
  | 'processing'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'refunded'
  | 'partial_refund';

/**
 * Payment result
 */
export interface PaymentResult {
  requestId: string;
  paymentId?: string;
  status: PaymentStatus;
  transactionRef?: string;
  receiptUrl?: string;
  completedAt?: Date;
  failedAt?: Date;
  failureReason?: string;
}

// ============================================================================
// Secrets
// ============================================================================

/**
 * Secret type/security level
 */
export type SecretType = 'minor' | 'critical' | 'user_owned';

/**
 * Secret storage request
 */
export interface SecretStoreRequest {
  userId: string;
  secretType: SecretType;
  name: string;
  description?: string;
  data: Uint8Array;
  metadata?: Record<string, unknown>;
  callbackUrl?: string;
  expiresIn?: number;
}

/**
 * Secret retrieval request
 */
export interface SecretRetrieveRequest {
  userId: string;
  secretId: string;
  purpose?: string;
  callbackUrl?: string;
  expiresIn?: number;
}

/**
 * Secret metadata
 */
export interface SecretMetadata {
  secretId: string;
  name: string;
  secretType: SecretType;
  description?: string;
  createdAt: Date;
  updatedAt?: Date;
}

/**
 * Secret result
 */
export interface SecretResult {
  requestId: string;
  status: RequestStatus;
  secretId?: string;
  data?: Uint8Array;
  metadata?: SecretMetadata[];
}

// ============================================================================
// Errors
// ============================================================================

/**
 * API error codes
 */
export type ErrorCode =
  | 'invalid_request'
  | 'unauthorized'
  | 'forbidden'
  | 'not_found'
  | 'conflict'
  | 'timeout'
  | 'user_offline'
  | 'contract_required'
  | 'capability_denied'
  | 'signature_invalid'
  | 'internal_error';

/**
 * API error
 */
export interface APIError {
  code: ErrorCode;
  message: string;
  details?: unknown;
}

// ============================================================================
// SDK Configuration
// ============================================================================

/**
 * SDK configuration options
 */
export interface VaultConfig {
  /** Base URL of the Service Vault API */
  baseUrl: string;

  /** API key for authentication */
  apiKey: string;

  /** Request timeout in milliseconds (default: 30000) */
  timeout?: number;

  /** Number of retry attempts (default: 3) */
  retries?: number;

  /** Custom HTTP headers */
  headers?: Record<string, string>;
}

/**
 * Webhook event types
 */
export type WebhookEventType =
  | 'auth.response'
  | 'authz.response'
  | 'data.response'
  | 'contract.created'
  | 'contract.cancelled'
  | 'call.accepted'
  | 'call.rejected'
  | 'call.ended'
  | 'call.missed'
  | 'payment.approved'
  | 'payment.denied'
  | 'payment.completed'
  | 'payment.failed'
  | 'secret.response';

/**
 * Webhook payload
 */
export interface WebhookPayload<T = unknown> {
  eventType: WebhookEventType;
  timestamp: Date;
  data: T;
}
