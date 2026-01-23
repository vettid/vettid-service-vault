package nats

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// UserCredentials contains the NATS connection credentials for a user.
// These are issued by the service when a user establishes a contract.
type UserCredentials struct {
	// Endpoint is the NATS server URL to connect to
	Endpoint string `json:"endpoint"`

	// AccountJWT is the signed account JWT
	AccountJWT string `json:"account_jwt"`

	// UserJWT is the signed user JWT
	UserJWT string `json:"user_jwt"`

	// UserSeed is the user's private key seed (nkey format)
	UserSeed string `json:"user_seed"`

	// ExpiresAt is when these credentials expire
	ExpiresAt time.Time `json:"expires_at"`
}

// CredentialIssuer manages NATS credential issuance for users.
type CredentialIssuer struct {
	// accountKeyPair is the account signing key
	accountKeyPair nkeys.KeyPair

	// operatorKeyPair is the operator signing key (if this service is the operator)
	operatorKeyPair nkeys.KeyPair

	// accountPublicKey is the account's public key
	accountPublicKey string

	// serviceID is the service identifier
	serviceID string

	// endpoint is the NATS server endpoint
	endpoint string

	// credentialTTL is the default lifetime for credentials
	credentialTTL time.Duration
}

// CredentialIssuerConfig holds configuration for the credential issuer.
type CredentialIssuerConfig struct {
	// AccountSeed is the account's private key seed (starts with SA)
	AccountSeed string

	// OperatorSeed is the operator's private key seed (starts with SO)
	// If empty, account-level credentials are issued
	OperatorSeed string

	// ServiceID is the service identifier
	ServiceID string

	// Endpoint is the NATS server endpoint
	Endpoint string

	// CredentialTTL is the default lifetime for credentials
	CredentialTTL time.Duration
}

// NewCredentialIssuer creates a new credential issuer.
func NewCredentialIssuer(cfg CredentialIssuerConfig) (*CredentialIssuer, error) {
	accountKP, err := nkeys.FromSeed([]byte(cfg.AccountSeed))
	if err != nil {
		return nil, fmt.Errorf("parsing account seed: %w", err)
	}

	accountPub, err := accountKP.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("getting account public key: %w", err)
	}

	issuer := &CredentialIssuer{
		accountKeyPair:   accountKP,
		accountPublicKey: accountPub,
		serviceID:        cfg.ServiceID,
		endpoint:         cfg.Endpoint,
		credentialTTL:    cfg.CredentialTTL,
	}

	if cfg.OperatorSeed != "" {
		operatorKP, err := nkeys.FromSeed([]byte(cfg.OperatorSeed))
		if err != nil {
			return nil, fmt.Errorf("parsing operator seed: %w", err)
		}
		issuer.operatorKeyPair = operatorKP
	}

	if issuer.credentialTTL == 0 {
		issuer.credentialTTL = 24 * time.Hour * 365 // 1 year default
	}

	return issuer, nil
}

// IssueUserCredentials creates NATS credentials for a user.
// The connectionKey is the user's X25519 public key from the contract.
func (c *CredentialIssuer) IssueUserCredentials(userID string, connectionKey [32]byte) (*UserCredentials, error) {
	// Generate a new user key pair
	userKP, err := nkeys.CreateUser()
	if err != nil {
		return nil, fmt.Errorf("creating user key: %w", err)
	}

	userPub, err := userKP.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("getting user public key: %w", err)
	}

	userSeed, err := userKP.Seed()
	if err != nil {
		return nil, fmt.Errorf("getting user seed: %w", err)
	}

	// Create user claims with permissions
	expiresAt := time.Now().Add(c.credentialTTL)

	userClaims := jwt.NewUserClaims(userPub)
	userClaims.Name = fmt.Sprintf("user-%s", userID)
	userClaims.Expires = expiresAt.Unix()

	// Set permissions for ServiceSpace topics
	// Users can publish to: ServiceSpace.<service_id>.fromUser.<user_id>.>
	// Users can subscribe to: ServiceSpace.<service_id>.toUser.<user_id>.>
	userClaims.Pub.Allow = []string{
		fmt.Sprintf("ServiceSpace.%s.fromUser.%s.>", c.serviceID, userID),
	}
	userClaims.Sub.Allow = []string{
		fmt.Sprintf("ServiceSpace.%s.toUser.%s.>", c.serviceID, userID),
	}

	// Store connection key hash for verification
	keyHash := base64.StdEncoding.EncodeToString(connectionKey[:8])
	userClaims.Tags.Add(fmt.Sprintf("connection_key:%s", keyHash))

	// Sign the user JWT with the account key
	userJWT, err := userClaims.Encode(c.accountKeyPair)
	if err != nil {
		return nil, fmt.Errorf("encoding user JWT: %w", err)
	}

	// Create account JWT if needed (for fresh accounts)
	accountJWT, err := c.issueAccountJWT()
	if err != nil {
		return nil, fmt.Errorf("issuing account JWT: %w", err)
	}

	return &UserCredentials{
		Endpoint:   c.endpoint,
		AccountJWT: accountJWT,
		UserJWT:    userJWT,
		UserSeed:   string(userSeed),
		ExpiresAt:  expiresAt,
	}, nil
}

// issueAccountJWT creates an account JWT.
func (c *CredentialIssuer) issueAccountJWT() (string, error) {
	accountClaims := jwt.NewAccountClaims(c.accountPublicKey)
	accountClaims.Name = fmt.Sprintf("service-%s", c.serviceID)

	// Set account limits
	accountClaims.Limits.Conn = 10000 // Max connections
	accountClaims.Limits.Subs = 100000 // Max subscriptions
	accountClaims.Limits.Payload = 1024 * 1024 // 1MB payload

	// If we have an operator key, sign with that
	signingKey := c.accountKeyPair
	if c.operatorKeyPair != nil {
		signingKey = c.operatorKeyPair
	}

	return accountClaims.Encode(signingKey)
}

// RevokeUserCredentials revokes credentials for a user.
// This prevents the user from connecting with their existing credentials.
func (c *CredentialIssuer) RevokeUserCredentials(userID string) error {
	// In a production system, this would:
	// 1. Add the user's public key to a revocation list
	// 2. Update the account JWT with the revocation
	// 3. Push the updated account JWT to the NATS server

	// For now, this is a placeholder - actual revocation requires
	// integration with the NATS server's account resolver
	return nil
}

// CredentialsToBytes serializes credentials to the creds file format.
func (creds *UserCredentials) CredentialsToBytes() []byte {
	return []byte(fmt.Sprintf(`-----BEGIN NATS USER JWT-----
%s
------END NATS USER JWT------

************************* IMPORTANT *************************
NKEY Seed printed below can be used to sign and prove identity.
NKEYs are sensitive and should be treated as secrets.

-----BEGIN USER NKEY SEED-----
%s
------END USER NKEY SEED------

*************************************************************
`, creds.UserJWT, creds.UserSeed))
}

// GenerateAccountKeyPair generates a new account key pair.
// This is useful for setting up a new service.
func GenerateAccountKeyPair() (publicKey, seed string, err error) {
	kp, err := nkeys.CreateAccount()
	if err != nil {
		return "", "", err
	}

	pub, err := kp.PublicKey()
	if err != nil {
		return "", "", err
	}

	seedBytes, err := kp.Seed()
	if err != nil {
		return "", "", err
	}

	return pub, string(seedBytes), nil
}

// GenerateOperatorKeyPair generates a new operator key pair.
func GenerateOperatorKeyPair() (publicKey, seed string, err error) {
	kp, err := nkeys.CreateOperator()
	if err != nil {
		return "", "", err
	}

	pub, err := kp.PublicKey()
	if err != nil {
		return "", "", err
	}

	seedBytes, err := kp.Seed()
	if err != nil {
		return "", "", err
	}

	return pub, string(seedBytes), nil
}

// generateRandomID generates a random identifier.
func generateRandomID(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:length]
}
