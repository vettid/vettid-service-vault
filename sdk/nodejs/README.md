# VettID Service Vault SDK for Node.js

TypeScript/JavaScript SDK for integrating with VettID Service Vault.

## Installation

```bash
npm install @vettid/service-vault-sdk
```

## Quick Start

```typescript
import {
  ServiceVaultClient,
  WebhookRouter,
  generateServiceIdentity
} from '@vettid/service-vault-sdk';

// 1. Generate service identity (do this once during setup)
const { identity, keyPair } = generateServiceIdentity('My Service', 'generic');
console.log('Service ID:', identity.serviceId);
// Store keyPair securely (e.g., in a secrets manager)

// 2. Create the vault client
const vault = new ServiceVaultClient({
  baseUrl: process.env.VAULT_URL || 'http://localhost:8080',
  apiKey: process.env.VAULT_API_KEY!,
});

// 3. Request user authentication
const requestId = await vault.requestAuth({
  userId: 'user123',
  purpose: 'Login to dashboard',
  callbackUrl: 'https://myservice.com/webhooks/vettid',
});

// 4. Handle the response via webhook
const webhooks = new WebhookRouter();

webhooks.on('auth.response', async (data) => {
  if (data.status === 'approved') {
    // User authenticated successfully!
    await createSession(data.userId, data.sessionKey);
  }
});

// Express example
app.post('/webhooks/vettid', express.json(), (req, res) => {
  webhooks.handle(req.body);
  res.sendStatus(200);
});
```

## Features

### Authentication & Authorization

```typescript
// Request authentication
const authId = await vault.requestAuth({
  userId: 'user123',
  purpose: 'Login',
  callbackUrl: 'https://myservice.com/webhook',
});

// Request authorization for an action
const authzId = await vault.requestAuthz({
  userId: 'user123',
  action: 'transfer_funds',
  resource: 'account:12345',
  callbackUrl: 'https://myservice.com/webhook',
});

// Check status (or wait for webhook)
const authStatus = await vault.getAuthRequest(authId);
```

### Voice/Video Calls

```typescript
// Initiate a video call
const { callId, expiresAt } = await vault.initiateCall({
  userId: 'user123',
  type: 'video',
  purpose: 'Support call',
  callbackUrl: 'https://myservice.com/webhook',
});

// Handle call events
webhooks.on('call.accepted', async (data) => {
  // User accepted, set up WebRTC connection
  console.log('Call accepted:', data.callId);
});

webhooks.on('call.rejected', async (data) => {
  console.log('Call rejected:', data.reason);
});

// End the call
await vault.endCall(callId, 'Call completed');
```

### Payments

```typescript
// Request payment
const { requestId } = await vault.requestPayment({
  userId: 'user123',
  amount: { amount: 1999, currency: 'USD' }, // $19.99
  description: 'Premium subscription',
  items: [
    {
      name: 'Premium Plan',
      quantity: 1,
      unitPrice: { amount: 1999, currency: 'USD' },
    },
  ],
  callbackUrl: 'https://myservice.com/webhook',
});

// Handle payment approval
webhooks.on('payment.approved', async (data) => {
  // Process the payment with your payment provider
  const result = await chargeCard(data.paymentId);

  if (result.success) {
    await vault.completePayment(data.requestId, result.transactionId);
  } else {
    await vault.failPayment(data.requestId, result.error);
  }
});
```

### Secrets Storage

Store sensitive data in the user's vault:

```typescript
// Store a secret (minor = auto-access after initial approval)
const { requestId } = await vault.storeSecret({
  userId: 'user123',
  secretType: 'minor',
  name: 'API Preferences',
  data: new TextEncoder().encode(JSON.stringify({ theme: 'dark' })),
  callbackUrl: 'https://myservice.com/webhook',
});

// Store a critical secret (requires password each access)
const { requestId: criticalId } = await vault.storeSecret({
  userId: 'user123',
  secretType: 'critical',
  name: 'Signing Key',
  data: keyBytes,
  callbackUrl: 'https://myservice.com/webhook',
});

// Retrieve a secret
const { requestId: retrieveId } = await vault.retrieveSecret({
  userId: 'user123',
  secretId: 'secret_abc123',
  purpose: 'Sign transaction',
  callbackUrl: 'https://myservice.com/webhook',
});
```

### Contract Management

```typescript
// Generate connection invite
const { inviteUrl, inviteCode } = await vault.generateInvite({
  offeringId: 'premium-tier',
  expiresIn: 3600, // 1 hour
});

// List active contracts
const contracts = await vault.listContracts({ status: 'active' });

// Cancel a contract
await vault.cancelContract(contractId, 'User requested cancellation');
```

## Cryptography

The SDK includes cryptographic utilities:

```typescript
import {
  generateServiceIdentity,
  sign,
  verify,
  encrypt,
  decrypt,
  signObject,
  verifyObject,
} from '@vettid/service-vault-sdk';

// Generate identity
const { identity, keyPair } = generateServiceIdentity('My Service', 'payment');

// Sign data
const signature = sign(data, keyPair.signingPrivateKey);
const valid = verify(data, signature, keyPair.signingPublicKey);

// Encrypt for a recipient
const { ciphertext, nonce, ephemeralPublicKey } = encrypt(
  message,
  recipientPublicKey
);

// Sign JSON objects (canonical serialization)
const objSignature = signObject({ foo: 'bar' }, keyPair.signingPrivateKey);
```

## Webhook Handling

The `WebhookRouter` provides type-safe webhook handling:

```typescript
const router = new WebhookRouter();

// Auth events
router.on('auth.response', (data) => { /* AuthResponse */ });
router.on('authz.response', (data) => { /* AuthzResponse */ });

// Call events
router.on('call.accepted', (data) => { /* CallAcceptedEvent */ });
router.on('call.rejected', (data) => { /* CallRejectedEvent */ });
router.on('call.ended', (data) => { /* CallEndedEvent */ });
router.on('call.missed', (data) => { /* CallMissedEvent */ });

// Payment events
router.on('payment.approved', (data) => { /* PaymentApprovedEvent */ });
router.on('payment.denied', (data) => { /* PaymentDeniedEvent */ });
router.on('payment.completed', (data) => { /* PaymentResult */ });
router.on('payment.failed', (data) => { /* PaymentResult */ });

// Secret events
router.on('secret.response', (data) => { /* SecretResult */ });

// Contract events
router.on('contract.created', (data) => { /* ConnectionContract */ });
router.on('contract.cancelled', (data) => { /* ConnectionContract */ });

// Catch-all for unhandled events
router.onAny((payload) => {
  console.log('Unhandled event:', payload.eventType);
});
```

## Error Handling

```typescript
import { VaultError } from '@vettid/service-vault-sdk';

try {
  await vault.requestAuth({ userId: 'user123', purpose: 'Login' });
} catch (error) {
  if (error instanceof VaultError) {
    console.error(`Error ${error.code}: ${error.message}`);
    // Handle specific error codes
    switch (error.code) {
      case 'contract_required':
        // User needs to connect first
        break;
      case 'capability_denied':
        // Contract doesn't allow this action
        break;
      case 'user_offline':
        // User is offline, consider offline grace period
        break;
    }
  }
}
```

## Configuration

```typescript
const vault = new ServiceVaultClient({
  baseUrl: 'https://vault.example.com',
  apiKey: 'sk_live_...',
  timeout: 30000,  // Request timeout (ms)
  retries: 3,      // Retry attempts
  headers: {       // Custom headers
    'X-Custom-Header': 'value',
  },
});
```

## License

MIT
