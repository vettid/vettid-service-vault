// Package config provides configuration loading for the Service Vault.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the Service Vault.
type Config struct {
	// Service identity
	ServiceName string `json:"service_name"`
	ServiceType string `json:"service_type"`

	// API server
	APIAddr string `json:"api_addr"`

	// Key storage
	KeystoreType string `json:"keystore_type"` // memory, file, kms
	KeystorePath string `json:"keystore_path"` // For file-based keystore
	KMSKeyARN    string `json:"kms_key_arn"`   // For KMS keystore

	// NATS configuration
	ServiceSpaceEndpoint string `json:"servicespace_endpoint"`
	ServiceSpaceCredsFile string `json:"servicespace_creds_file"`
	MessageSpaceEndpoint string `json:"messagespace_endpoint"`
	MessageSpaceCredsFile string `json:"messagespace_creds_file"`

	// DynamoDB
	DynamoDBTableName string `json:"dynamodb_table_name"`
	DynamoDBEndpoint  string `json:"dynamodb_endpoint"` // For local development

	// Request timeouts
	DefaultTimeout     time.Duration `json:"default_timeout"`
	DefaultOfflineGrace time.Duration `json:"default_offline_grace"`

	// Environment
	Environment string `json:"environment"` // development, staging, production
}

// DefaultConfig returns a Config with sensible defaults for development.
func DefaultConfig() *Config {
	return &Config{
		ServiceName:          "service-vault",
		ServiceType:          "generic",
		APIAddr:              ":8080",
		KeystoreType:         "memory",
		ServiceSpaceEndpoint: "nats://localhost:4222",
		MessageSpaceEndpoint: "nats://localhost:4223",
		DynamoDBTableName:    "vettid-service-vault-contracts",
		DefaultTimeout:       5 * time.Minute,
		DefaultOfflineGrace:  24 * time.Hour,
		Environment:          "development",
	}
}

// Load loads configuration from environment variables and optional config file.
// Environment variables take precedence over file values.
func Load(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	// Load from file if provided
	if configPath != "" {
		if err := cfg.loadFromFile(configPath); err != nil {
			return nil, fmt.Errorf("loading config file: %w", err)
		}
	}

	// Override with environment variables
	cfg.loadFromEnv()

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

func (c *Config) loadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, c)
}

func (c *Config) loadFromEnv() {
	if v := os.Getenv("SERVICE_NAME"); v != "" {
		c.ServiceName = v
	}
	if v := os.Getenv("SERVICE_TYPE"); v != "" {
		c.ServiceType = v
	}
	if v := os.Getenv("API_ADDR"); v != "" {
		c.APIAddr = v
	}
	if v := os.Getenv("KEYSTORE_TYPE"); v != "" {
		c.KeystoreType = v
	}
	if v := os.Getenv("KEYSTORE_PATH"); v != "" {
		c.KeystorePath = v
	}
	if v := os.Getenv("KMS_KEY_ARN"); v != "" {
		c.KMSKeyARN = v
	}
	if v := os.Getenv("SERVICESPACE_ENDPOINT"); v != "" {
		c.ServiceSpaceEndpoint = v
	}
	if v := os.Getenv("SERVICESPACE_CREDS_FILE"); v != "" {
		c.ServiceSpaceCredsFile = v
	}
	if v := os.Getenv("MESSAGESPACE_ENDPOINT"); v != "" {
		c.MessageSpaceEndpoint = v
	}
	if v := os.Getenv("MESSAGESPACE_CREDS_FILE"); v != "" {
		c.MessageSpaceCredsFile = v
	}
	if v := os.Getenv("DYNAMODB_TABLE_NAME"); v != "" {
		c.DynamoDBTableName = v
	}
	if v := os.Getenv("DYNAMODB_ENDPOINT"); v != "" {
		c.DynamoDBEndpoint = v
	}
	if v := os.Getenv("DEFAULT_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.DefaultTimeout = d
		}
	}
	if v := os.Getenv("DEFAULT_OFFLINE_GRACE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.DefaultOfflineGrace = d
		}
	}
	if v := os.Getenv("ENVIRONMENT"); v != "" {
		c.Environment = v
	}
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}
	if c.APIAddr == "" {
		return fmt.Errorf("api_addr is required")
	}

	validKeystoreTypes := map[string]bool{"memory": true, "file": true, "kms": true}
	if !validKeystoreTypes[c.KeystoreType] {
		return fmt.Errorf("invalid keystore_type: %s (must be memory, file, or kms)", c.KeystoreType)
	}

	if c.KeystoreType == "file" && c.KeystorePath == "" {
		return fmt.Errorf("keystore_path is required when keystore_type is file")
	}
	if c.KeystoreType == "kms" && c.KMSKeyARN == "" {
		return fmt.Errorf("kms_key_arn is required when keystore_type is kms")
	}

	validEnvs := map[string]bool{"development": true, "staging": true, "production": true}
	if !validEnvs[c.Environment] {
		return fmt.Errorf("invalid environment: %s", c.Environment)
	}

	return nil
}

// IsDevelopment returns true if running in development environment.
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

// IsProduction returns true if running in production environment.
func (c *Config) IsProduction() bool {
	return c.Environment == "production"
}

// getEnvInt is a helper to get an integer from environment variable.
func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}
