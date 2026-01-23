/**
 * Webhook handling utilities for VettID Service Vault
 */

import {
  WebhookEventType,
  WebhookPayload,
  AuthResponse,
  AuthzResponse,
  DataResponse,
  ConnectionContract,
  CallResult,
  PaymentResult,
  SecretResult,
} from './types';

/**
 * Event handler function type
 */
export type WebhookHandler<T = unknown> = (data: T) => void | Promise<void>;

/**
 * Webhook event handlers
 */
export interface WebhookHandlers {
  'auth.response'?: WebhookHandler<AuthResponse>;
  'authz.response'?: WebhookHandler<AuthzResponse>;
  'data.response'?: WebhookHandler<DataResponse>;
  'contract.created'?: WebhookHandler<ConnectionContract>;
  'contract.cancelled'?: WebhookHandler<ConnectionContract>;
  'call.accepted'?: WebhookHandler<CallAcceptedEvent>;
  'call.rejected'?: WebhookHandler<CallRejectedEvent>;
  'call.ended'?: WebhookHandler<CallEndedEvent>;
  'call.missed'?: WebhookHandler<CallMissedEvent>;
  'payment.approved'?: WebhookHandler<PaymentApprovedEvent>;
  'payment.denied'?: WebhookHandler<PaymentDeniedEvent>;
  'payment.completed'?: WebhookHandler<PaymentResult>;
  'payment.failed'?: WebhookHandler<PaymentResult>;
  'secret.response'?: WebhookHandler<SecretResult>;
}

/**
 * Call accepted webhook event
 */
export interface CallAcceptedEvent {
  requestId: string;
  callId: string;
  userId: string;
  type: 'voice' | 'video';
}

/**
 * Call rejected webhook event
 */
export interface CallRejectedEvent {
  requestId: string;
  userId: string;
  reason?: string;
}

/**
 * Call ended webhook event
 */
export interface CallEndedEvent {
  callId: string;
  userId: string;
  duration?: number;
  reason?: string;
}

/**
 * Call missed webhook event
 */
export interface CallMissedEvent {
  requestId: string;
  userId: string;
}

/**
 * Payment approved webhook event
 */
export interface PaymentApprovedEvent {
  requestId: string;
  paymentId: string;
  userId: string;
  amount: number;
  currency: string;
  method: string;
}

/**
 * Payment denied webhook event
 */
export interface PaymentDeniedEvent {
  requestId: string;
  userId: string;
  reason?: string;
}

/**
 * WebhookRouter handles incoming webhook events from VettID Service Vault.
 *
 * @example
 * ```typescript
 * const router = new WebhookRouter();
 *
 * router.on('auth.response', async (data) => {
 *   if (data.status === 'approved') {
 *     // User authenticated successfully
 *     await createSession(data.userId);
 *   }
 * });
 *
 * router.on('payment.approved', async (data) => {
 *   // Process the payment
 *   await chargeCard(data.paymentId);
 * });
 *
 * // In your HTTP handler:
 * app.post('/webhooks/vettid', async (req, res) => {
 *   await router.handle(req.body);
 *   res.status(200).send('OK');
 * });
 * ```
 */
export class WebhookRouter {
  private handlers: WebhookHandlers = {};
  private catchAllHandler?: WebhookHandler<WebhookPayload>;

  /**
   * Register a handler for a specific event type.
   */
  on<K extends keyof WebhookHandlers>(
    eventType: K,
    handler: WebhookHandlers[K]
  ): this {
    this.handlers[eventType] = handler as any;
    return this;
  }

  /**
   * Register a catch-all handler for unhandled events.
   */
  onAny(handler: WebhookHandler<WebhookPayload>): this {
    this.catchAllHandler = handler;
    return this;
  }

  /**
   * Handle an incoming webhook payload.
   *
   * @param payload - The webhook payload from VettID
   */
  async handle(payload: WebhookPayload | string): Promise<void> {
    const data: WebhookPayload = typeof payload === 'string'
      ? JSON.parse(payload)
      : payload;

    // Convert timestamp if needed
    if (typeof data.timestamp === 'string') {
      data.timestamp = new Date(data.timestamp);
    }

    const handler = this.handlers[data.eventType as keyof WebhookHandlers];
    if (handler) {
      const eventData = this.transformEventData(data.eventType as WebhookEventType, data.data);
      await (handler as WebhookHandler)(eventData);
    } else if (this.catchAllHandler) {
      await this.catchAllHandler(data);
    }
  }

  /**
   * Transform raw webhook data into typed event objects.
   */
  private transformEventData(eventType: WebhookEventType, data: any): any {
    switch (eventType) {
      case 'auth.response':
        return {
          requestId: data.request_id,
          status: data.status,
          userId: data.user_id,
          timestamp: new Date(data.timestamp),
          sessionKey: data.session_key,
        } as AuthResponse;

      case 'authz.response':
        return {
          requestId: data.request_id,
          status: data.status,
          userId: data.user_id,
          action: data.action,
          resource: data.resource,
          timestamp: new Date(data.timestamp),
          expiresAt: data.expires_at ? new Date(data.expires_at) : undefined,
        } as AuthzResponse;

      case 'call.accepted':
        return {
          requestId: data.request_id,
          callId: data.call_id,
          userId: data.user_id,
          type: data.type,
        } as CallAcceptedEvent;

      case 'call.rejected':
        return {
          requestId: data.request_id,
          userId: data.user_id,
          reason: data.reason,
        } as CallRejectedEvent;

      case 'call.ended':
        return {
          callId: data.call_id,
          userId: data.user_id,
          duration: data.duration,
          reason: data.reason,
        } as CallEndedEvent;

      case 'call.missed':
        return {
          requestId: data.request_id,
          userId: data.user_id,
        } as CallMissedEvent;

      case 'payment.approved':
        return {
          requestId: data.request_id,
          paymentId: data.payment_id,
          userId: data.user_id,
          amount: data.amount,
          currency: data.currency,
          method: data.method,
        } as PaymentApprovedEvent;

      case 'payment.denied':
        return {
          requestId: data.request_id,
          userId: data.user_id,
          reason: data.reason,
        } as PaymentDeniedEvent;

      case 'secret.response':
        return {
          requestId: data.request_id,
          status: data.status,
          secretId: data.secret_id,
          // Note: actual secret data is not included in webhooks for security
        } as SecretResult;

      default:
        return data;
    }
  }
}

/**
 * Express middleware for handling VettID webhooks.
 *
 * @example
 * ```typescript
 * import express from 'express';
 * import { createWebhookMiddleware, WebhookRouter } from '@vettid/service-vault-sdk';
 *
 * const app = express();
 * const router = new WebhookRouter();
 *
 * router.on('auth.response', (data) => {
 *   console.log('Auth response:', data);
 * });
 *
 * app.post('/webhooks/vettid', express.json(), createWebhookMiddleware(router));
 * ```
 */
export function createWebhookMiddleware(router: WebhookRouter) {
  return async (req: any, res: any, next?: any) => {
    try {
      await router.handle(req.body);
      res.status(200).send('OK');
    } catch (error) {
      console.error('Webhook handling error:', error);
      res.status(500).send('Internal Server Error');
      if (next) next(error);
    }
  };
}
