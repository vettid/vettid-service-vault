/**
 * VettID Service Vault Client
 *
 * Main client for interacting with the VettID Service Vault API.
 */

import {
  VaultConfig,
  AuthRequest,
  AuthResponse,
  AuthzRequest,
  AuthzResponse,
  DataRequest,
  DataResponse,
  Notification,
  NotificationResult,
  CallRequest,
  CallResult,
  PaymentRequest,
  PaymentResult,
  SecretStoreRequest,
  SecretRetrieveRequest,
  SecretResult,
  ConnectionContract,
  APIError,
  RequestStatus,
} from './types';

/**
 * HTTP client response
 */
interface HttpResponse<T> {
  ok: boolean;
  status: number;
  data?: T;
  error?: APIError;
}

/**
 * VettID Service Vault Client
 *
 * @example
 * ```typescript
 * const vault = new ServiceVaultClient({
 *   baseUrl: 'http://localhost:8080',
 *   apiKey: 'your-api-key',
 * });
 *
 * // Request authentication
 * const result = await vault.requestAuth({
 *   userId: 'user123',
 *   purpose: 'Login to dashboard',
 * });
 * ```
 */
export class ServiceVaultClient {
  private config: Required<VaultConfig>;

  constructor(config: VaultConfig) {
    this.config = {
      baseUrl: config.baseUrl.replace(/\/$/, ''), // Remove trailing slash
      apiKey: config.apiKey,
      timeout: config.timeout ?? 30000,
      retries: config.retries ?? 3,
      headers: config.headers ?? {},
    };
  }

  // ============================================================================
  // Authentication
  // ============================================================================

  /**
   * Request authentication from a user.
   *
   * @param request - Authentication request parameters
   * @returns Promise resolving to the request ID (response comes via webhook)
   *
   * @example
   * ```typescript
   * const requestId = await vault.requestAuth({
   *   userId: 'user123',
   *   purpose: 'Login to dashboard',
   *   callbackUrl: 'https://myservice.com/webhooks/vettid',
   * });
   * ```
   */
  async requestAuth(request: Omit<AuthRequest, 'requestId' | 'expiresAt'> & {
    expiresIn?: number;
  }): Promise<string> {
    const response = await this.post<{ request_id: string }>('/api/v1/auth/request', {
      user_id: request.userId,
      purpose: request.purpose,
      context: request.context,
      expires_in: request.expiresIn,
      offline_grace: request.offlineGrace,
      callback_url: request.callbackUrl,
    });

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }

    return response.data.request_id;
  }

  /**
   * Get the status of an authentication request.
   */
  async getAuthRequest(requestId: string): Promise<AuthResponse> {
    const response = await this.get<{
      request_id: string;
      status: RequestStatus;
      user_id: string;
      timestamp: string;
      session_key?: string;
    }>(`/api/v1/auth/request/${requestId}`);

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'not_found', message: 'Request not found' });
    }

    return {
      requestId: response.data.request_id,
      status: response.data.status,
      userId: response.data.user_id,
      timestamp: new Date(response.data.timestamp),
      sessionKey: response.data.session_key,
    };
  }

  // ============================================================================
  // Authorization
  // ============================================================================

  /**
   * Request authorization for an action from a user.
   *
   * @example
   * ```typescript
   * const requestId = await vault.requestAuthz({
   *   userId: 'user123',
   *   action: 'transfer_funds',
   *   resource: 'account:12345',
   *   callbackUrl: 'https://myservice.com/webhooks/vettid',
   * });
   * ```
   */
  async requestAuthz(request: Omit<AuthzRequest, 'requestId' | 'expiresAt'> & {
    expiresIn?: number;
  }): Promise<string> {
    const response = await this.post<{ request_id: string }>('/api/v1/authz/request', {
      user_id: request.userId,
      action: request.action,
      resource: request.resource,
      context: request.context,
      expires_in: request.expiresIn,
      offline_grace: request.offlineGrace,
      callback_url: request.callbackUrl,
    });

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }

    return response.data.request_id;
  }

  /**
   * Get the status of an authorization request.
   */
  async getAuthzRequest(requestId: string): Promise<AuthzResponse> {
    const response = await this.get<{
      request_id: string;
      status: RequestStatus;
      user_id: string;
      action: string;
      resource: string;
      timestamp: string;
      expires_at?: string;
    }>(`/api/v1/authz/request/${requestId}`);

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'not_found', message: 'Request not found' });
    }

    return {
      requestId: response.data.request_id,
      status: response.data.status,
      userId: response.data.user_id,
      action: response.data.action,
      resource: response.data.resource,
      timestamp: new Date(response.data.timestamp),
      expiresAt: response.data.expires_at ? new Date(response.data.expires_at) : undefined,
    };
  }

  // ============================================================================
  // Contracts
  // ============================================================================

  /**
   * List all active contracts.
   */
  async listContracts(options?: {
    status?: string;
    limit?: number;
    offset?: number;
  }): Promise<ConnectionContract[]> {
    const params = new URLSearchParams();
    if (options?.status) params.set('status', options.status);
    if (options?.limit) params.set('limit', options.limit.toString());
    if (options?.offset) params.set('offset', options.offset.toString());

    const query = params.toString();
    const url = `/api/v1/contracts${query ? `?${query}` : ''}`;

    const response = await this.get<{ contracts: any[] }>(url);

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }

    return response.data.contracts.map(this.mapContract);
  }

  /**
   * Get a specific contract by ID.
   */
  async getContract(contractId: string): Promise<ConnectionContract> {
    const response = await this.get<any>(`/api/v1/contracts/${contractId}`);

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'not_found', message: 'Contract not found' });
    }

    return this.mapContract(response.data);
  }

  /**
   * Generate a connection invite URL/QR code.
   */
  async generateInvite(options?: {
    offeringId?: string;
    expiresIn?: number;
  }): Promise<{ inviteUrl: string; inviteCode: string; expiresAt: Date }> {
    const response = await this.post<{
      invite_url: string;
      invite_code: string;
      expires_at: string;
    }>('/api/v1/contracts/invite', {
      offering_id: options?.offeringId,
      expires_in: options?.expiresIn,
    });

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }

    return {
      inviteUrl: response.data.invite_url,
      inviteCode: response.data.invite_code,
      expiresAt: new Date(response.data.expires_at),
    };
  }

  /**
   * Cancel a contract.
   */
  async cancelContract(contractId: string, reason?: string): Promise<void> {
    const response = await this.delete(`/api/v1/contracts/${contractId}`, {
      reason,
    });

    if (!response.ok) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }
  }

  // ============================================================================
  // Calls
  // ============================================================================

  /**
   * Initiate a voice or video call to a user.
   *
   * @example
   * ```typescript
   * const result = await vault.initiateCall({
   *   userId: 'user123',
   *   type: 'video',
   *   purpose: 'Support call',
   *   callbackUrl: 'https://myservice.com/webhooks/vettid',
   * });
   * ```
   */
  async initiateCall(request: CallRequest): Promise<{ callId: string; expiresAt: Date }> {
    const response = await this.post<{
      call_id: string;
      status: string;
      expires_at: string;
    }>('/api/v1/call/initiate', {
      user_id: request.userId,
      type: request.type,
      purpose: request.purpose,
      context: request.context,
      ice_servers: request.iceServers,
      offer: request.offer,
      expires_in: request.expiresIn,
      callback_url: request.callbackUrl,
    });

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }

    return {
      callId: response.data.call_id,
      expiresAt: new Date(response.data.expires_at),
    };
  }

  /**
   * Get the status of a call.
   */
  async getCallStatus(callId: string): Promise<CallResult> {
    const response = await this.get<any>(`/api/v1/call/${callId}`);

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'not_found', message: 'Call not found' });
    }

    return {
      callId: response.data.call_id,
      requestId: response.data.request_id,
      status: response.data.status,
      answer: response.data.answer,
      startedAt: response.data.started_at ? new Date(response.data.started_at) : undefined,
      connectedAt: response.data.connected_at ? new Date(response.data.connected_at) : undefined,
      endedAt: response.data.ended_at ? new Date(response.data.ended_at) : undefined,
      duration: response.data.duration_seconds,
    };
  }

  /**
   * End an active call.
   */
  async endCall(callId: string, reason?: string): Promise<void> {
    const response = await this.post(`/api/v1/call/${callId}/end`, { reason });

    if (!response.ok) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }
  }

  // ============================================================================
  // Payments
  // ============================================================================

  /**
   * Request a payment from a user.
   *
   * @example
   * ```typescript
   * const requestId = await vault.requestPayment({
   *   userId: 'user123',
   *   amount: { amount: 1999, currency: 'USD' },
   *   description: 'Premium subscription',
   *   callbackUrl: 'https://myservice.com/webhooks/vettid',
   * });
   * ```
   */
  async requestPayment(request: PaymentRequest): Promise<{
    requestId: string;
    expiresAt: Date;
  }> {
    const response = await this.post<{
      request_id: string;
      status: string;
      expires_at: string;
    }>('/api/v1/payment/request', {
      user_id: request.userId,
      amount: request.amount,
      description: request.description,
      merchant_info: request.merchantInfo,
      items: request.items,
      allowed_methods: request.allowedMethods,
      recurring_info: request.recurringInfo,
      callback_url: request.callbackUrl,
      expires_in: request.expiresIn,
      metadata: request.metadata,
    });

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }

    return {
      requestId: response.data.request_id,
      expiresAt: new Date(response.data.expires_at),
    };
  }

  /**
   * Get the status of a payment request.
   */
  async getPaymentStatus(requestId: string): Promise<PaymentResult> {
    const response = await this.get<any>(`/api/v1/payment/request/${requestId}`);

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'not_found', message: 'Payment not found' });
    }

    return {
      requestId: response.data.request_id,
      paymentId: response.data.payment_id,
      status: response.data.status,
      transactionRef: response.data.transaction_id,
      receiptUrl: response.data.receipt_url,
      completedAt: response.data.completed_at ? new Date(response.data.completed_at) : undefined,
      failedAt: response.data.failed_at ? new Date(response.data.failed_at) : undefined,
      failureReason: response.data.failure_reason,
    };
  }

  /**
   * Mark a payment as completed.
   */
  async completePayment(requestId: string, transactionId: string, receiptUrl?: string): Promise<void> {
    const response = await this.post(`/api/v1/payment/request/${requestId}/complete`, {
      transaction_id: transactionId,
      receipt_url: receiptUrl,
    });

    if (!response.ok) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }
  }

  /**
   * Mark a payment as failed.
   */
  async failPayment(requestId: string, reason: string): Promise<void> {
    const response = await this.post(`/api/v1/payment/request/${requestId}/fail`, {
      reason,
    });

    if (!response.ok) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }
  }

  /**
   * Refund a payment.
   */
  async refundPayment(requestId: string, amount?: number, reason?: string): Promise<void> {
    const response = await this.post(`/api/v1/payment/request/${requestId}/refund`, {
      amount,
      reason,
    });

    if (!response.ok) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }
  }

  // ============================================================================
  // Secrets
  // ============================================================================

  /**
   * Store a secret in the user's vault.
   *
   * @example
   * ```typescript
   * const requestId = await vault.storeSecret({
   *   userId: 'user123',
   *   secretType: 'minor',
   *   name: 'API Preferences',
   *   data: new TextEncoder().encode(JSON.stringify({ theme: 'dark' })),
   *   callbackUrl: 'https://myservice.com/webhooks/vettid',
   * });
   * ```
   */
  async storeSecret(request: SecretStoreRequest): Promise<{
    requestId: string;
    expiresAt: Date;
  }> {
    const response = await this.post<{
      request_id: string;
      status: string;
      expires_at: string;
    }>('/api/v1/secrets/store', {
      user_id: request.userId,
      secret_type: request.secretType,
      name: request.name,
      description: request.description,
      data: Buffer.from(request.data).toString('base64'),
      metadata: request.metadata,
      callback_url: request.callbackUrl,
      expires_in: request.expiresIn,
    });

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }

    return {
      requestId: response.data.request_id,
      expiresAt: new Date(response.data.expires_at),
    };
  }

  /**
   * Retrieve a secret from the user's vault.
   */
  async retrieveSecret(request: SecretRetrieveRequest): Promise<{
    requestId: string;
    expiresAt: Date;
  }> {
    const response = await this.post<{
      request_id: string;
      status: string;
      expires_at: string;
    }>('/api/v1/secrets/retrieve', {
      user_id: request.userId,
      secret_id: request.secretId,
      purpose: request.purpose,
      callback_url: request.callbackUrl,
      expires_in: request.expiresIn,
    });

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }

    return {
      requestId: response.data.request_id,
      expiresAt: new Date(response.data.expires_at),
    };
  }

  /**
   * Delete a secret from the user's vault.
   */
  async deleteSecret(userId: string, secretId: string, callbackUrl?: string): Promise<{
    requestId: string;
    expiresAt: Date;
  }> {
    const params = new URLSearchParams({ user_id: userId });
    if (callbackUrl) params.set('callback_url', callbackUrl);

    const response = await this.delete<{
      request_id: string;
      status: string;
      expires_at: string;
    }>(`/api/v1/secrets/${secretId}?${params.toString()}`);

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }

    return {
      requestId: response.data.request_id,
      expiresAt: new Date(response.data.expires_at),
    };
  }

  /**
   * List secrets stored for a user.
   */
  async listSecrets(userId: string, options?: {
    secretType?: string;
    callbackUrl?: string;
  }): Promise<{ requestId: string; expiresAt: Date }> {
    const params = new URLSearchParams({ user_id: userId });
    if (options?.secretType) params.set('secret_type', options.secretType);
    if (options?.callbackUrl) params.set('callback_url', options.callbackUrl);

    const response = await this.get<{
      request_id: string;
      status: string;
      expires_at: string;
    }>(`/api/v1/secrets?${params.toString()}`);

    if (!response.ok || !response.data) {
      throw new VaultError(response.error ?? { code: 'internal_error', message: 'Unknown error' });
    }

    return {
      requestId: response.data.request_id,
      expiresAt: new Date(response.data.expires_at),
    };
  }

  // ============================================================================
  // Health
  // ============================================================================

  /**
   * Check if the vault is healthy.
   */
  async health(): Promise<boolean> {
    try {
      const response = await this.get<{ status: string }>('/health');
      return response.ok && response.data?.status === 'ok';
    } catch {
      return false;
    }
  }

  // ============================================================================
  // Private HTTP Methods
  // ============================================================================

  private async get<T>(path: string): Promise<HttpResponse<T>> {
    return this.request<T>('GET', path);
  }

  private async post<T>(path: string, body?: unknown): Promise<HttpResponse<T>> {
    return this.request<T>('POST', path, body);
  }

  private async delete<T = void>(path: string, body?: unknown): Promise<HttpResponse<T>> {
    return this.request<T>('DELETE', path, body);
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown
  ): Promise<HttpResponse<T>> {
    const url = `${this.config.baseUrl}${path}`;
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${this.config.apiKey}`,
      ...this.config.headers,
    };

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), this.config.timeout);

    try {
      const response = await fetch(url, {
        method,
        headers,
        body: body ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      });

      const data = await response.json().catch(() => null);

      if (!response.ok) {
        return {
          ok: false,
          status: response.status,
          error: data as APIError ?? {
            code: 'internal_error',
            message: `HTTP ${response.status}`,
          },
        };
      }

      return {
        ok: true,
        status: response.status,
        data: data as T,
      };
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        return {
          ok: false,
          status: 0,
          error: { code: 'timeout', message: 'Request timed out' },
        };
      }
      throw error;
    } finally {
      clearTimeout(timeout);
    }
  }

  private mapContract(data: any): ConnectionContract {
    return {
      contractId: data.contract_id,
      userId: data.user_id,
      serviceId: data.service_id,
      offeringId: data.offering_id,
      offering: {
        offeringId: data.offering?.offering_id,
        name: data.offering?.name,
        description: data.offering?.description,
        capabilities: data.offering?.capabilities ?? [],
        requiredData: data.offering?.required_data,
        pricing: data.offering?.pricing,
        termsUrl: data.offering?.terms_url,
        termsHash: data.offering?.terms_hash,
      },
      status: data.status,
      createdAt: new Date(data.created_at),
      activatedAt: data.activated_at ? new Date(data.activated_at) : undefined,
      cancelledAt: data.cancelled_at ? new Date(data.cancelled_at) : undefined,
    };
  }
}

/**
 * Error thrown by the Service Vault client
 */
export class VaultError extends Error {
  public readonly code: string;
  public readonly details?: unknown;

  constructor(error: APIError) {
    super(error.message);
    this.name = 'VaultError';
    this.code = error.code;
    this.details = error.details;
  }
}
