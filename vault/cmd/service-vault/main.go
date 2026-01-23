// Package main provides the entrypoint for the VettID Service Vault.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/vettid/vettid-service-vault/vault/internal/api"
	"github.com/vettid/vettid-service-vault/vault/internal/config"
	"github.com/vettid/vettid-service-vault/vault/internal/contract"
	"github.com/vettid/vettid-service-vault/vault/internal/handler"
	"github.com/vettid/vettid-service-vault/vault/internal/identity"
	"github.com/vettid/vettid-service-vault/vault/internal/keystore"
	"github.com/vettid/vettid-service-vault/vault/internal/nats"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("VettID Service Vault %s (commit: %s, built: %s)\n", version, commit, buildDate)
		os.Exit(0)
	}

	// Set up structured logging
	logLevel := slog.LevelInfo
	if os.Getenv("DEBUG") == "true" {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("starting VettID Service Vault",
		"version", version,
		"service_name", cfg.ServiceName,
		"environment", cfg.Environment,
	)

	// Create context that listens for shutdown signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run the service
	errChan := make(chan error, 1)
	go func() {
		errChan <- run(ctx, cfg, logger)
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		slog.Info("received shutdown signal", "signal", sig)
	case err := <-errChan:
		if err != nil {
			slog.Error("service error", "error", err)
		}
	}

	// Trigger graceful shutdown
	cancel()

	// Give components time to shut down gracefully
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Wait for shutdown to complete
	select {
	case <-shutdownCtx.Done():
		slog.Warn("shutdown timed out, forcing exit")
	case err := <-errChan:
		if err != nil {
			slog.Error("shutdown error", "error", err)
			os.Exit(1)
		}
	}

	slog.Info("service stopped")
}

// run initializes and runs all service components.
func run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	// 1. Initialize keystore
	ks, err := initKeystore(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize keystore: %w", err)
	}
	logger.Info("keystore initialized", "type", cfg.KeystoreType)

	// 2. Load or generate service identity
	svcIdentity, encKey, err := loadOrGenerateIdentity(ctx, cfg, ks, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize identity: %w", err)
	}
	logger.Info("service identity loaded",
		"service_id", svcIdentity.ServiceID,
		"service_name", svcIdentity.ServiceName,
	)

	// 3. Initialize NATS clients (optional, only if configured)
	var serviceSpace *nats.ServiceSpaceClient
	var messageSpace *nats.MessageSpaceClient

	if cfg.ServiceSpaceEndpoint != "" {
		serviceSpaceCfg := nats.DefaultClientConfig()
		serviceSpaceCfg.Endpoint = cfg.ServiceSpaceEndpoint
		serviceSpaceCfg.ServiceID = svcIdentity.ServiceID
		serviceSpaceCfg.Name = cfg.ServiceName + "-servicespace"

		if cfg.ServiceSpaceCredsFile != "" {
			creds, err := os.ReadFile(cfg.ServiceSpaceCredsFile)
			if err != nil {
				logger.Warn("failed to read ServiceSpace credentials, continuing without NATS", "error", err)
			} else {
				serviceSpaceCfg.Credentials = creds
			}
		}

		serviceSpace = nats.NewServiceSpaceClient(serviceSpaceCfg, encKey)
		if err := serviceSpace.Connect(); err != nil {
			logger.Warn("failed to connect to ServiceSpace, continuing without NATS", "error", err)
			serviceSpace = nil
		} else {
			defer serviceSpace.Close()
			logger.Info("connected to ServiceSpace NATS", "endpoint", cfg.ServiceSpaceEndpoint)
		}
	}

	if cfg.MessageSpaceEndpoint != "" {
		messageSpaceCfg := nats.DefaultClientConfig()
		messageSpaceCfg.Endpoint = cfg.MessageSpaceEndpoint
		messageSpaceCfg.ServiceID = svcIdentity.ServiceID
		messageSpaceCfg.Name = cfg.ServiceName + "-messagespace"

		if cfg.MessageSpaceCredsFile != "" {
			creds, err := os.ReadFile(cfg.MessageSpaceCredsFile)
			if err != nil {
				logger.Warn("failed to read MessageSpace credentials, continuing without MessageSpace", "error", err)
			} else {
				messageSpaceCfg.Credentials = creds
			}
		}

		messageSpace = nats.NewMessageSpaceClient(messageSpaceCfg, encKey)
		if err := messageSpace.Connect(); err != nil {
			logger.Warn("failed to connect to MessageSpace, continuing without MessageSpace", "error", err)
			messageSpace = nil
		} else {
			defer messageSpace.Close()
			logger.Info("connected to MessageSpace NATS", "endpoint", cfg.MessageSpaceEndpoint)
		}
	}

	// 4. Initialize contract store (optional, only if configured)
	var contractStore contract.Store
	var negotiator *contract.Negotiator

	if cfg.DynamoDBTableName != "" {
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to load AWS config: %w", err)
		}

		// Use local endpoint for development
		var dynamoClient *dynamodb.Client
		if cfg.DynamoDBEndpoint != "" {
			dynamoClient = dynamodb.NewFromConfig(awsCfg, func(o *dynamodb.Options) {
				o.BaseEndpoint = &cfg.DynamoDBEndpoint
			})
		} else {
			dynamoClient = dynamodb.NewFromConfig(awsCfg)
		}

		contractStore, err = contract.NewDynamoDBStore(contract.DynamoDBStoreConfig{
			Client:    dynamoClient,
			TableName: cfg.DynamoDBTableName,
			ServiceID: svcIdentity.ServiceID,
		})
		if err != nil {
			return fmt.Errorf("failed to create contract store: %w", err)
		}
		logger.Info("contract store initialized", "table", cfg.DynamoDBTableName)

		// Initialize negotiator
		negotiator, err = contract.NewNegotiator(contract.NegotiatorConfig{
			Store:           contractStore,
			Keystore:        ks,
			ServiceIdentity: svcIdentity,
			SigningKeyID:    "service-signing",
			EncryptionKeyID: "service-encryption",
		})
		if err != nil {
			logger.Warn("failed to create negotiator", "error", err)
		} else {
			logger.Info("contract negotiator initialized")
		}
	}

	// 5. Initialize handler engine (requires contract store for full functionality)
	var engine *handler.Engine
	var authHandler *handler.AuthHandler
	var authzHandler *handler.AuthzHandler

	if contractStore != nil {
		engine, err = handler.NewEngine(handler.EngineConfig{
			Contracts:      contractStore,
			Keystore:       ks,
			ServiceSpace:   serviceSpace,
			MessageSpace:   messageSpace,
			ServiceID:      svcIdentity.ServiceID,
			DefaultTimeout: cfg.DefaultTimeout,
		})
		if err != nil {
			return fmt.Errorf("failed to create handler engine: %w", err)
		}

		// Create handlers with engine reference
		authHandler = handler.NewAuthHandler(handler.AuthHandlerConfig{
			Engine:       engine,
			Timeout:      cfg.DefaultTimeout,
			OfflineGrace: cfg.DefaultOfflineGrace,
		})

		authzHandler = handler.NewAuthzHandler(handler.AuthzHandlerConfig{
			Engine:       engine,
			Timeout:      cfg.DefaultTimeout,
			OfflineGrace: cfg.DefaultOfflineGrace,
		})

		// Register handlers with engine for NATS message routing
		engine.RegisterHandler(authHandler)
		engine.RegisterHandler(authzHandler)

		// Start engine if we have ServiceSpace
		if serviceSpace != nil {
			if err := engine.Start(); err != nil {
				logger.Warn("failed to start handler engine", "error", err)
			} else {
				logger.Info("handler engine started")
			}
		}
	} else {
		// Create minimal handlers for API-only mode (without engine)
		// These handlers will return errors for auth/authz requests but allow health checks
		authHandler = handler.NewAuthHandler(handler.AuthHandlerConfig{
			Timeout:      cfg.DefaultTimeout,
			OfflineGrace: cfg.DefaultOfflineGrace,
		})
		authzHandler = handler.NewAuthzHandler(handler.AuthzHandlerConfig{
			Timeout:      cfg.DefaultTimeout,
			OfflineGrace: cfg.DefaultOfflineGrace,
		})
		logger.Warn("running in API-only mode without contract store")
	}

	// 6. Initialize API server
	server, err := api.NewServer(api.ServerConfig{
		Config:       cfg,
		Identity:     svcIdentity,
		Contracts:    contractStore,
		Negotiator:   negotiator,
		AuthHandler:  authHandler,
		AuthzHandler: authzHandler,
		Logger:       logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create API server: %w", err)
	}

	// Start API server in background
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting API server", "addr", cfg.APIAddr)
		if err := server.Start(cfg.APIAddr); err != nil {
			serverErr <- err
		}
	}()

	logger.Info("service vault initialized and running",
		"api_addr", cfg.APIAddr,
		"keystore_type", cfg.KeystoreType,
		"service_id", svcIdentity.ServiceID,
		"has_contracts", contractStore != nil,
		"has_nats", serviceSpace != nil,
	)

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		logger.Info("shutting down...")
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("error during server shutdown", "error", err)
	}

	// NATS connections are closed via defer above
	logger.Info("service vault shutdown complete")

	return nil
}

// initKeystore creates a keystore based on configuration.
func initKeystore(ctx context.Context, cfg *config.Config, logger *slog.Logger) (keystore.KeyStore, error) {
	switch cfg.KeystoreType {
	case "memory":
		logger.Warn("using in-memory keystore (not for production)")
		return keystore.NewMemoryKeyStore(), nil

	case "file":
		if cfg.KeystorePath == "" {
			return nil, errors.New("keystore_path required for file keystore")
		}
		logger.Warn("using file keystore (not for production)", "path", cfg.KeystorePath)
		return keystore.NewFileKeyStore(cfg.KeystorePath)

	case "kms":
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS config: %w", err)
		}
		return keystore.NewKMSKeyStore(awsCfg, cfg.KMSKeyARN)

	default:
		return nil, fmt.Errorf("unknown keystore type: %s", cfg.KeystoreType)
	}
}

// loadOrGenerateIdentity loads existing identity or generates a new one.
// Returns the identity and the encryption private key (needed for NATS clients).
func loadOrGenerateIdentity(ctx context.Context, cfg *config.Config, ks keystore.KeyStore, logger *slog.Logger) (*identity.ServiceIdentity, [32]byte, error) {
	var encKey [32]byte

	// Try to load existing identity from keystore
	signingPrivKey, err := ks.GetSigningKey("service-signing")
	if err == nil {
		// Identity exists, reconstruct it
		encKey, err = ks.GetEncryptionKey("service-encryption")
		if err != nil {
			return nil, encKey, fmt.Errorf("signing key exists but encryption key missing: %w", err)
		}

		// Create identity from private keys (derives public keys internally)
		svcIdentity := identity.FromPrivateKeys(
			signingPrivKey,
			encKey,
			cfg.ServiceName,
			types.ServiceType(cfg.ServiceType),
			cfg.ServiceSpaceEndpoint,
		)

		return svcIdentity, encKey, nil
	}

	// Generate new identity
	logger.Info("generating new service identity")
	svcIdentity, keyPair, err := identity.GenerateIdentity(
		cfg.ServiceName,
		types.ServiceType(cfg.ServiceType),
		cfg.ServiceSpaceEndpoint,
	)
	if err != nil {
		return nil, encKey, fmt.Errorf("failed to generate identity: %w", err)
	}

	// Store keys
	if err := ks.StoreSigningKey("service-signing", keyPair.SigningPrivateKey); err != nil {
		return nil, encKey, fmt.Errorf("failed to store signing key: %w", err)
	}
	if err := ks.StoreEncryptionKey("service-encryption", keyPair.EncryptionPrivateKey); err != nil {
		return nil, encKey, fmt.Errorf("failed to store encryption key: %w", err)
	}

	logger.Info("new service identity generated and stored",
		"service_id", svcIdentity.ServiceID,
	)

	return svcIdentity, keyPair.EncryptionPrivateKey, nil
}
