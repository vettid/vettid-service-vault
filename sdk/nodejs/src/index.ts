/**
 * VettID Service Vault SDK for Node.js/TypeScript
 *
 * @packageDocumentation
 *
 * @example
 * ```typescript
 * import {
 *   ServiceVaultClient,
 *   WebhookRouter,
 *   generateServiceIdentity
 * } from '@vettid/service-vault-sdk';
 *
 * // Generate service identity (do this once, store securely)
 * const { identity, keyPair } = generateServiceIdentity('My Service', 'generic');
 *
 * // Create vault client
 * const vault = new ServiceVaultClient({
 *   baseUrl: 'http://localhost:8080',
 *   apiKey: 'your-api-key',
 * });
 *
 * // Request authentication
 * const requestId = await vault.requestAuth({
 *   userId: 'user123',
 *   purpose: 'Login to dashboard',
 *   callbackUrl: 'https://myservice.com/webhooks/vettid',
 * });
 *
 * // Handle webhooks
 * const webhooks = new WebhookRouter();
 * webhooks.on('auth.response', async (data) => {
 *   if (data.status === 'approved') {
 *     await createUserSession(data.userId);
 *   }
 * });
 * ```
 */

// Main client
export { ServiceVaultClient, VaultError } from './client';

// Types
export * from './types';

// Cryptography
export {
  generateServiceIdentity,
  deriveServiceId,
  sign,
  verify,
  encrypt,
  decrypt,
  generateNonce,
  randomBytes,
  encodeBase64,
  decodeBase64,
  encodeBase58,
  decodeBase58,
  canonicalize,
  signObject,
  verifyObject,
} from './crypto';

// Webhooks
export {
  WebhookRouter,
  createWebhookMiddleware,
  type WebhookHandler,
  type WebhookHandlers,
  type CallAcceptedEvent,
  type CallRejectedEvent,
  type CallEndedEvent,
  type CallMissedEvent,
  type PaymentApprovedEvent,
  type PaymentDeniedEvent,
} from './webhook';
