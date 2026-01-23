# VettID Service Vault SDK for Python

Python SDK for integrating with VettID Service Vault. Enables services to authenticate users, request authorization, handle payments, manage secrets, and more.

## Installation

```bash
pip install vettid-service-vault
```

## Quick Start

```python
import asyncio
from vettid_vault import ServiceVaultClient, RequestStatus

async def main():
    async with ServiceVaultClient(
        base_url="https://vault.yourservice.com",
        api_key="your-api-key"
    ) as client:
        # Request authentication from a user
        request_id = await client.request_auth(
            user_id="user123",
            purpose="Login to dashboard",
            callback_url="https://yourservice.com/webhook"
        )
        print(f"Auth request: {request_id}")

        # Check status
        response = await client.get_auth_request(request_id)
        if response.status == RequestStatus.APPROVED:
            print(f"User authenticated! Session: {response.session_key}")

asyncio.run(main())
```

## Features

- **Authentication**: Request user authentication with purpose disclosure
- **Authorization**: Request specific action permissions
- **Contracts**: Manage service-user connection contracts
- **Calls**: Initiate voice/video calls to users
- **Payments**: Request payments with detailed receipts
- **Secrets**: Store and retrieve encrypted secrets in user vaults

## API Reference

### ServiceVaultClient

The main client for interacting with Service Vault.

```python
from vettid_vault import ServiceVaultClient

client = ServiceVaultClient(
    base_url="https://vault.yourservice.com",
    api_key="your-api-key",
    timeout=30.0,  # Request timeout in seconds
    retries=3,     # Number of retries
)
```

#### Authentication

```python
# Request authentication
request_id = await client.request_auth(
    user_id="user123",
    purpose="Login to dashboard",
    context={"ip": "192.168.1.1"},  # Optional context
    expires_in=300,                  # Seconds until expiry
    offline_grace=5000,              # Offline grace in ms
    callback_url="https://..."       # Webhook URL
)

# Get auth request status
response = await client.get_auth_request(request_id)
print(response.status)  # RequestStatus.PENDING, APPROVED, DENIED, etc.
```

#### Authorization

```python
# Request authorization for an action
request_id = await client.request_authz(
    user_id="user123",
    action="transfer_funds",
    resource="account:checking",
    context={"amount": 500},
    callback_url="https://..."
)

# Get authorization status
response = await client.get_authz_request(request_id)
```

#### Contracts

```python
# List contracts
contracts = await client.list_contracts(status="active", limit=10)

# Get specific contract
contract = await client.get_contract("contract-id")

# Generate connection invite
invite = await client.generate_invite(
    offering_id="premium-plan",
    expires_in=86400
)
print(invite["invite_url"])

# Cancel a contract
await client.cancel_contract("contract-id", reason="User requested")
```

#### Calls

```python
# Initiate a call
result = await client.initiate_call(
    user_id="user123",
    call_type="video",
    purpose="Support call",
    callback_url="https://..."
)
print(result["call_id"])

# Get call status
status = await client.get_call_status(result["call_id"])

# End a call
await client.end_call(result["call_id"], reason="Call completed")
```

#### Payments

```python
from vettid_vault import Money

# Request payment
result = await client.request_payment(
    user_id="user123",
    amount=Money(amount=1999, currency="USD"),  # $19.99
    description="Monthly subscription",
    items=[
        {"name": "Pro Plan", "quantity": 1, "unit_price": 1999}
    ],
    callback_url="https://..."
)

# Get payment status
status = await client.get_payment_status(result["request_id"])

# Mark payment as complete
await client.complete_payment(
    request_id=result["request_id"],
    transaction_id="txn_123",
    receipt_url="https://..."
)

# Refund payment
await client.refund_payment(
    request_id=result["request_id"],
    amount=500,  # Partial refund
    reason="Service issue"
)
```

#### Secrets

```python
# Store a secret in user's vault
result = await client.store_secret(
    user_id="user123",
    secret_type="api_key",
    name="GitHub Token",
    data=b"ghp_xxxxxxxxxxxx",
    description="Personal access token",
    callback_url="https://..."
)

# Retrieve a secret
result = await client.retrieve_secret(
    user_id="user123",
    secret_id="secret-id",
    purpose="Deploy application",
    callback_url="https://..."
)

# Delete a secret
await client.delete_secret(
    user_id="user123",
    secret_id="secret-id"
)
```

### Webhook Handling

Handle asynchronous responses from Service Vault.

```python
from vettid_vault import WebhookRouter, RequestStatus
from vettid_vault.webhook import AuthEvent, PaymentEvent

router = WebhookRouter(signing_secret="your-webhook-secret")

@router.on_auth
async def handle_auth(event: AuthEvent):
    if event.status == RequestStatus.APPROVED:
        print(f"User {event.user_id} authenticated")
        # Create session, etc.

@router.on_payment
async def handle_payment(event: PaymentEvent):
    if event.status == PaymentStatus.COMPLETED:
        print(f"Payment {event.payment_id} completed")
        # Fulfill order, etc.

# In your web framework (e.g., FastAPI)
from fastapi import FastAPI, Request, HTTPException

app = FastAPI()

@app.post("/webhook")
async def webhook(request: Request):
    signature = request.headers.get("X-Vault-Signature")
    body = await request.body()

    try:
        await router.handle(body, signature)
        return {"status": "ok"}
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
```

### Cryptography

Generate service identities and handle cryptographic operations.

```python
from vettid_vault import (
    generate_service_identity,
    derive_service_id,
    sign,
    verify,
    encrypt,
    decrypt,
)
from vettid_vault.types import ServiceType

# Generate a new service identity
identity, keypair = generate_service_identity(
    service_name="My Service",
    service_type=ServiceType.COMMERCE
)
print(f"Service ID: {identity.service_id}")

# Sign data
signature = sign(b"Hello, World!", keypair.signing_private_key)

# Verify signature
is_valid = verify(
    b"Hello, World!",
    signature,
    keypair.signing_public_key
)

# Encrypt for a recipient
ciphertext, nonce, ephemeral_pk = encrypt(
    b"Secret message",
    recipient_public_key
)

# Decrypt
plaintext = decrypt(
    ciphertext,
    nonce,
    sender_public_key,
    keypair.encryption_private_key
)
```

## Types

The SDK provides typed dataclasses for all responses:

```python
from vettid_vault import (
    ServiceType,        # GENERIC, PAYMENT, IDENTITY, etc.
    ContractStatus,     # PENDING, ACTIVE, PAUSED, etc.
    RequestStatus,      # PENDING, APPROVED, DENIED, etc.
    CallStatus,         # INITIATING, RINGING, CONNECTED, etc.
    PaymentStatus,      # PENDING, COMPLETED, FAILED, etc.
    SecretType,         # MINOR, CRITICAL, USER_OWNED
    AuthResponse,
    AuthzResponse,
    ConnectionContract,
    CallResult,
    PaymentResult,
    Money,
)
```

## Error Handling

```python
from vettid_vault import ServiceVaultClient, VaultError

async with ServiceVaultClient(...) as client:
    try:
        await client.request_auth(...)
    except VaultError as e:
        print(f"Error code: {e.code}")
        print(f"Message: {e}")
        print(f"Details: {e.details}")
```

## Development

```bash
# Install dev dependencies
pip install -e ".[dev]"

# Run tests
pytest

# Type checking
mypy src/

# Linting
ruff check src/
```

## License

MIT
