# VettID Service Vault SDK for Go

Go SDK for integrating with VettID Service Vault. Enables services to authenticate users, request authorization, handle payments, manage secrets, and more.

## Installation

```bash
go get github.com/vettid/vettid-service-vault/sdk/go
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    vettid "github.com/vettid/vettid-service-vault/sdk/go"
)

func main() {
    client := vettid.NewClient(
        "https://vault.yourservice.com",
        "your-api-key",
    )

    ctx := context.Background()

    // Request authentication from a user
    requestID, err := client.RequestAuth(ctx, "user123", "Login to dashboard", &vettid.AuthRequestOptions{
        CallbackURL: "https://yourservice.com/webhook",
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Auth request: %s\n", requestID)

    // Check status
    response, err := client.GetAuthRequest(ctx, requestID)
    if err != nil {
        log.Fatal(err)
    }
    if response.Status == vettid.RequestStatusApproved {
        fmt.Printf("User authenticated! Session: %s\n", response.SessionKey)
    }
}
```

## Features

- **Authentication**: Request user authentication with purpose disclosure
- **Authorization**: Request specific action permissions
- **Contracts**: Manage service-user connection contracts
- **Calls**: Initiate voice/video calls to users
- **Payments**: Request payments with detailed receipts
- **Secrets**: Store and retrieve encrypted secrets in user vaults

## API Reference

### Client

Create a new client with default configuration:

```go
client := vettid.NewClient(baseURL, apiKey)
```

Or with custom configuration:

```go
client := vettid.NewClientWithConfig(&vettid.VaultConfig{
    BaseURL: "https://vault.yourservice.com",
    APIKey:  "your-api-key",
    Timeout: 30 * time.Second,
    Retries: 3,
    Headers: map[string]string{
        "X-Custom-Header": "value",
    },
})
```

### Authentication

```go
// Request authentication
requestID, err := client.RequestAuth(ctx, "user123", "Login to dashboard", &vettid.AuthRequestOptions{
    Context:      map[string]interface{}{"ip": "192.168.1.1"},
    ExpiresIn:    300,  // seconds
    OfflineGrace: 5000, // milliseconds
    CallbackURL:  "https://...",
})

// Get auth request status
response, err := client.GetAuthRequest(ctx, requestID)
// response.Status: RequestStatusPending, RequestStatusApproved, RequestStatusDenied, etc.
```

### Authorization

```go
// Request authorization for an action
requestID, err := client.RequestAuthz(ctx, "user123", "transfer_funds", "account:checking", &vettid.AuthzRequestOptions{
    Context:     map[string]interface{}{"amount": 500},
    CallbackURL: "https://...",
})

// Get authorization status
response, err := client.GetAuthzRequest(ctx, requestID)
```

### Contracts

```go
// List contracts
contracts, err := client.ListContracts(ctx, &vettid.ListContractsOptions{
    Status: "active",
    Limit:  10,
})

// Get specific contract
contract, err := client.GetContract(ctx, "contract-id")

// Generate connection invite
invite, err := client.GenerateInvite(ctx, "premium-plan", 86400)
fmt.Println(invite.InviteURL)

// Cancel a contract
err := client.CancelContract(ctx, "contract-id", "User requested")
```

### Calls

```go
// Initiate a call
result, err := client.InitiateCall(ctx, "user123", vettid.CallTypeVideo, &vettid.CallOptions{
    Purpose:     "Support call",
    CallbackURL: "https://...",
})
fmt.Println(result.CallID)

// Get call status
status, err := client.GetCallStatus(ctx, result.CallID)

// End a call
err := client.EndCall(ctx, result.CallID, "Call completed")
```

### Payments

```go
// Request payment
result, err := client.RequestPayment(ctx, "user123", vettid.Money{
    Amount:   1999, // $19.99
    Currency: "USD",
}, "Monthly subscription", &vettid.PaymentOptions{
    Items: []map[string]interface{}{
        {"name": "Pro Plan", "quantity": 1, "unit_price": 1999},
    },
    CallbackURL: "https://...",
})

// Get payment status
status, err := client.GetPaymentStatus(ctx, result.RequestID)

// Mark payment as complete
err := client.CompletePayment(ctx, result.RequestID, "txn_123", "https://receipt.url")

// Refund payment
err := client.RefundPayment(ctx, result.RequestID, 500, "Service issue") // Partial refund
```

### Secrets

```go
// Store a secret in user's vault
result, err := client.StoreSecret(ctx, "user123", "api_key", "GitHub Token", []byte("ghp_xxx"), &vettid.StoreSecretOptions{
    Description: "Personal access token",
    CallbackURL: "https://...",
})

// Retrieve a secret
result, err := client.RetrieveSecret(ctx, "user123", "secret-id", &vettid.RetrieveSecretOptions{
    Purpose:     "Deploy application",
    CallbackURL: "https://...",
})

// Delete a secret
result, err := client.DeleteSecret(ctx, "user123", "secret-id", "")
```

### Webhook Handling

Handle asynchronous responses from Service Vault:

```go
package main

import (
    "fmt"
    "io"
    "net/http"

    vettid "github.com/vettid/vettid-service-vault/sdk/go"
)

func main() {
    router := vettid.NewWebhookRouter("your-webhook-secret")

    router.OnAuth(func(event *vettid.AuthEvent) error {
        if event.Status == vettid.RequestStatusApproved {
            fmt.Printf("User %s authenticated\n", event.UserID)
            // Create session, etc.
        }
        return nil
    })

    router.OnPayment(func(event *vettid.PaymentEvent) error {
        if event.Status == vettid.PaymentStatusCompleted {
            fmt.Printf("Payment %s completed\n", event.PaymentID)
            // Fulfill order, etc.
        }
        return nil
    })

    http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
        signature := r.Header.Get("X-Vault-Signature")
        body, _ := io.ReadAll(r.Body)

        if err := router.Handle(body, signature); err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }
        w.WriteHeader(http.StatusOK)
    })

    http.ListenAndServe(":8080", nil)
}
```

### Cryptography

Generate service identities and handle cryptographic operations:

```go
package main

import (
    "fmt"

    vettid "github.com/vettid/vettid-service-vault/sdk/go"
)

func main() {
    // Generate a new service identity
    identity, keypair, err := vettid.GenerateServiceIdentity("My Service", vettid.ServiceTypeCommerce)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Service ID: %s\n", identity.ServiceID)

    // Sign data
    data := []byte("Hello, World!")
    signature := vettid.Sign(data, keypair.SigningPrivateKey)

    // Verify signature
    isValid := vettid.Verify(data, signature, keypair.SigningPublicKey)
    fmt.Printf("Signature valid: %v\n", isValid)

    // Encrypt for a recipient
    msg, err := vettid.Encrypt([]byte("Secret message"), recipientPublicKey, nil)
    if err != nil {
        panic(err)
    }

    // Decrypt
    plaintext, ok := vettid.Decrypt(msg, senderPublicKey, keypair.EncryptionPrivateKey)
    if ok {
        fmt.Printf("Decrypted: %s\n", plaintext)
    }

    // Sign an object (canonical JSON)
    obj := map[string]interface{}{
        "action": "transfer",
        "amount": 100,
    }
    sig, err := vettid.SignObject(obj, keypair.SigningPrivateKey)
    if err != nil {
        panic(err)
    }

    // Verify object signature
    valid, err := vettid.VerifyObject(obj, sig, keypair.SigningPublicKey)
    fmt.Printf("Object signature valid: %v\n", valid)
}
```

## Types

The SDK provides typed constants and structs:

```go
// Service types
vettid.ServiceTypeGeneric
vettid.ServiceTypePayment
vettid.ServiceTypeIdentity
vettid.ServiceTypeCommerce
vettid.ServiceTypeHealthcare
vettid.ServiceTypeFinance

// Contract status
vettid.ContractStatusPending
vettid.ContractStatusActive
vettid.ContractStatusPaused
vettid.ContractStatusCancelled
vettid.ContractStatusExpired

// Request status
vettid.RequestStatusPending
vettid.RequestStatusApproved
vettid.RequestStatusDenied
vettid.RequestStatusExpired
vettid.RequestStatusOfflineApproved

// Call status
vettid.CallStatusInitiating
vettid.CallStatusRinging
vettid.CallStatusConnected
vettid.CallStatusEnded
// ... etc.

// Payment status
vettid.PaymentStatusPending
vettid.PaymentStatusProcessing
vettid.PaymentStatusCompleted
vettid.PaymentStatusFailed
// ... etc.
```

## Error Handling

```go
result, err := client.RequestAuth(ctx, userID, purpose, nil)
if err != nil {
    if apiErr, ok := err.(*vettid.APIError); ok {
        fmt.Printf("Error code: %s\n", apiErr.Code)
        fmt.Printf("Message: %s\n", apiErr.Message)
        fmt.Printf("Details: %v\n", apiErr.Details)
    } else {
        // Network or other error
        fmt.Printf("Error: %v\n", err)
    }
}
```

## License

MIT
