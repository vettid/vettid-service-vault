// Package domain provides DNS domain validation for VettID Service Vault.
//
// Services can claim ownership of a domain by adding a DNS TXT record:
//
//	_vettid-service.<domain> TXT "vettid-service-id=<service_id>"
//
// This allows users to verify that a service legitimately represents a domain.
package domain

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Common errors
var (
	ErrDomainEmpty       = errors.New("domain is empty")
	ErrNoTXTRecord       = errors.New("no TXT record found")
	ErrServiceIDMismatch = errors.New("service ID does not match TXT record")
	ErrInvalidTXTFormat  = errors.New("invalid TXT record format")
	ErrLookupFailed      = errors.New("DNS lookup failed")
)

// Validator validates domain ownership via DNS TXT records.
type Validator struct {
	resolver Resolver
	timeout  time.Duration
}

// Resolver is an interface for DNS resolution (allows mocking in tests).
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// DefaultResolver uses net.Resolver for DNS lookups.
type DefaultResolver struct {
	resolver *net.Resolver
}

// NewDefaultResolver creates a resolver using the system DNS.
func NewDefaultResolver() *DefaultResolver {
	return &DefaultResolver{
		resolver: net.DefaultResolver,
	}
}

// LookupTXT performs a DNS TXT record lookup.
func (r *DefaultResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return r.resolver.LookupTXT(ctx, name)
}

// ValidatorConfig holds configuration for the domain validator.
type ValidatorConfig struct {
	Resolver Resolver
	Timeout  time.Duration
}

// NewValidator creates a new domain validator.
func NewValidator(cfg ValidatorConfig) *Validator {
	resolver := cfg.Resolver
	if resolver == nil {
		resolver = NewDefaultResolver()
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &Validator{
		resolver: resolver,
		timeout:  timeout,
	}
}

// ValidationResult contains the result of a domain validation attempt.
type ValidationResult struct {
	Domain      string
	ServiceID   string
	Verified    bool
	TXTRecords  []string
	Error       error
	VerifiedAt  time.Time
}

// Validate checks if the domain has a valid VettID service TXT record.
//
// The expected TXT record format is:
//
//	_vettid-service.<domain> TXT "vettid-service-id=<service_id>"
//
// For example, for domain "example.com" and service_id "3Kj9mNxPqRsT5vWy":
//
//	_vettid-service.example.com TXT "vettid-service-id=3Kj9mNxPqRsT5vWy"
func (v *Validator) Validate(ctx context.Context, domain string, serviceID string) (*ValidationResult, error) {
	if domain == "" {
		return nil, ErrDomainEmpty
	}

	// Normalize domain (remove trailing dot, lowercase)
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))

	result := &ValidationResult{
		Domain:    domain,
		ServiceID: serviceID,
		Verified:  false,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	// Build the TXT record name
	txtName := "_vettid-service." + domain

	// Lookup TXT records
	records, err := v.resolver.LookupTXT(ctx, txtName)
	if err != nil {
		// Check if it's a "not found" error
		if isNotFoundError(err) {
			result.Error = ErrNoTXTRecord
			return result, ErrNoTXTRecord
		}
		result.Error = fmt.Errorf("%w: %v", ErrLookupFailed, err)
		return result, result.Error
	}

	result.TXTRecords = records

	// Look for a matching record
	expectedValue := "vettid-service-id=" + serviceID

	for _, record := range records {
		// TXT records may be split across multiple strings, join them
		record = strings.TrimSpace(record)

		if record == expectedValue {
			result.Verified = true
			result.VerifiedAt = time.Now().UTC()
			return result, nil
		}

		// Check if it's a vettid record but with wrong service ID
		if strings.HasPrefix(record, "vettid-service-id=") {
			result.Error = ErrServiceIDMismatch
			return result, ErrServiceIDMismatch
		}
	}

	result.Error = ErrNoTXTRecord
	return result, ErrNoTXTRecord
}

// ValidateAsync performs validation asynchronously and returns a channel.
func (v *Validator) ValidateAsync(ctx context.Context, domain string, serviceID string) <-chan *ValidationResult {
	ch := make(chan *ValidationResult, 1)

	go func() {
		defer close(ch)
		result, _ := v.Validate(ctx, domain, serviceID)
		ch <- result
	}()

	return ch
}

// GenerateTXTRecord returns the TXT record value that should be added.
func GenerateTXTRecord(serviceID string) string {
	return "vettid-service-id=" + serviceID
}

// GenerateTXTRecordName returns the full TXT record name for a domain.
func GenerateTXTRecordName(domain string) string {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	return "_vettid-service." + domain
}

// Instructions returns human-readable instructions for setting up domain verification.
func Instructions(domain, serviceID string) string {
	recordName := GenerateTXTRecordName(domain)
	recordValue := GenerateTXTRecord(serviceID)

	return fmt.Sprintf(`To verify ownership of %s, add the following DNS TXT record:

Name:  %s
Type:  TXT
Value: %s

Example bind zone file entry:
%s. IN TXT "%s"

After adding the record, DNS propagation may take up to 48 hours.
You can verify the record using: dig TXT %s
`, domain, recordName, recordValue, recordName, recordValue, recordName)
}

// notFoundError is an interface for DNS errors that indicate "not found".
type notFoundError interface {
	error
	IsNotFound() bool
}

// isNotFoundError checks if an error indicates "not found".
func isNotFoundError(err error) bool {
	// Check for net.DNSError first
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return true
	}

	// Check for any error with IsNotFound method (for mocks)
	var nfe notFoundError
	if errors.As(err, &nfe) && nfe.IsNotFound() {
		return true
	}

	return false
}
