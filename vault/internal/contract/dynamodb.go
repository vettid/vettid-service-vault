package contract

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// DynamoDBStore implements the Store interface using DynamoDB.
//
// Table structure:
// - Primary key: service_id (PK), contract_id (SK)
// - GSI UserIndex: user_id (PK), created_at (SK)
// - GSI StatusIndex: service_id (PK), status (SK)
type DynamoDBStore struct {
	client    *dynamodb.Client
	tableName string
	serviceID string // For tenant isolation
}

// DynamoDBStoreConfig holds configuration for the DynamoDB store.
type DynamoDBStoreConfig struct {
	Client    *dynamodb.Client
	TableName string
	ServiceID string
}

// NewDynamoDBStore creates a new DynamoDB-backed contract store.
func NewDynamoDBStore(cfg DynamoDBStoreConfig) (*DynamoDBStore, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("DynamoDB client is required")
	}
	if cfg.TableName == "" {
		return nil, fmt.Errorf("table name is required")
	}
	if cfg.ServiceID == "" {
		return nil, fmt.Errorf("service ID is required")
	}

	return &DynamoDBStore{
		client:    cfg.Client,
		tableName: cfg.TableName,
		serviceID: cfg.ServiceID,
	}, nil
}

// contractItem is the DynamoDB item representation of a contract.
type contractItem struct {
	ServiceID string                    `dynamodbav:"service_id"`
	ContractID string                   `dynamodbav:"contract_id"`
	UserID    string                    `dynamodbav:"user_id"`
	Status    types.ContractStatus      `dynamodbav:"status"`
	Data      *SignedConnectionContract `dynamodbav:"data"`
	CreatedAt string                    `dynamodbav:"created_at"`
	TTL       int64                     `dynamodbav:"ttl,omitempty"`
}

// SaveContract stores a new contract.
func (s *DynamoDBStore) SaveContract(ctx context.Context, contract *SignedConnectionContract) error {
	if contract.ServiceID != s.serviceID {
		return fmt.Errorf("contract service_id mismatch: expected %s, got %s", s.serviceID, contract.ServiceID)
	}

	item := contractItem{
		ServiceID:  s.serviceID,
		ContractID: contract.ContractID,
		UserID:     contract.UserID,
		Status:     contract.Status,
		Data:       contract,
		CreatedAt:  contract.CreatedAt.Format(time.RFC3339),
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("marshaling contract: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(contract_id)"),
	})

	if err != nil {
		var ccf *ddbtypes.ConditionalCheckFailedException
		if stderrors.As(err, &ccf) {
			return ErrContractExists
		}
		return fmt.Errorf("saving contract: %w", err)
	}

	return nil
}

// GetContract retrieves a contract by ID.
func (s *DynamoDBStore) GetContract(ctx context.Context, contractID string) (*SignedConnectionContract, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]ddbtypes.AttributeValue{
			"service_id":  &ddbtypes.AttributeValueMemberS{Value: s.serviceID},
			"contract_id": &ddbtypes.AttributeValueMemberS{Value: contractID},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("getting contract: %w", err)
	}

	if result.Item == nil {
		return nil, ErrContractNotFound
	}

	var item contractItem
	if err := attributevalue.UnmarshalMap(result.Item, &item); err != nil {
		return nil, fmt.Errorf("unmarshaling contract: %w", err)
	}

	return item.Data, nil
}

// GetContractByUser retrieves a user's active contract.
func (s *DynamoDBStore) GetContractByUser(ctx context.Context, userID string) (*SignedConnectionContract, error) {
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("UserIndex"),
		KeyConditionExpression: aws.String("user_id = :uid"),
		FilterExpression:       aws.String("#status = :active"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":uid":    &ddbtypes.AttributeValueMemberS{Value: userID},
			":active": &ddbtypes.AttributeValueMemberS{Value: string(types.ContractStatusActive)},
		},
		Limit:            aws.Int32(1),
		ScanIndexForward: aws.Bool(false), // Most recent first
	})

	if err != nil {
		return nil, fmt.Errorf("querying contracts: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, ErrContractNotFound
	}

	var item contractItem
	if err := attributevalue.UnmarshalMap(result.Items[0], &item); err != nil {
		return nil, fmt.Errorf("unmarshaling contract: %w", err)
	}

	return item.Data, nil
}

// UpdateContract updates an existing contract.
func (s *DynamoDBStore) UpdateContract(ctx context.Context, contract *SignedConnectionContract) error {
	if contract.ServiceID != s.serviceID {
		return fmt.Errorf("contract service_id mismatch")
	}

	item := contractItem{
		ServiceID:  s.serviceID,
		ContractID: contract.ContractID,
		UserID:     contract.UserID,
		Status:     contract.Status,
		Data:       contract,
		CreatedAt:  contract.CreatedAt.Format(time.RFC3339),
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("marshaling contract: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_exists(contract_id)"),
	})

	if err != nil {
		var ccf *ddbtypes.ConditionalCheckFailedException
		if stderrors.As(err, &ccf) {
			return ErrContractNotFound
		}
		return fmt.Errorf("updating contract: %w", err)
	}

	return nil
}

// UpdateContractStatus updates only the status of a contract.
func (s *DynamoDBStore) UpdateContractStatus(ctx context.Context, contractID string, status types.ContractStatus) error {
	contract, err := s.GetContract(ctx, contractID)
	if err != nil {
		return err
	}

	contract.Status = status
	now := time.Now().UTC()

	switch status {
	case types.ContractStatusActive:
		contract.ActivatedAt = &now
	case types.ContractStatusCancelled:
		contract.CancelledAt = &now
	}

	return s.UpdateContract(ctx, contract)
}

// ListContracts retrieves contracts matching the filter.
func (s *DynamoDBStore) ListContracts(ctx context.Context, filter ContractFilter) (*ContractListResult, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	var items []map[string]ddbtypes.AttributeValue
	var nextCursor string

	if filter.UserID != "" {
		// Query by user using GSI
		input := &dynamodb.QueryInput{
			TableName:              aws.String(s.tableName),
			IndexName:              aws.String("UserIndex"),
			KeyConditionExpression: aws.String("user_id = :uid"),
			ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
				":uid": &ddbtypes.AttributeValueMemberS{Value: filter.UserID},
			},
			Limit:            aws.Int32(int32(limit)),
			ScanIndexForward: aws.Bool(false),
		}

		if filter.Status != nil {
			input.FilterExpression = aws.String("#status = :status")
			input.ExpressionAttributeNames = map[string]string{"#status": "status"}
			input.ExpressionAttributeValues[":status"] = &ddbtypes.AttributeValueMemberS{Value: string(*filter.Status)}
		}

		result, err := s.client.Query(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("querying contracts: %w", err)
		}
		items = result.Items
	} else {
		// Query by service_id (primary index)
		input := &dynamodb.QueryInput{
			TableName:              aws.String(s.tableName),
			KeyConditionExpression: aws.String("service_id = :sid"),
			ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
				":sid": &ddbtypes.AttributeValueMemberS{Value: s.serviceID},
			},
			Limit:            aws.Int32(int32(limit)),
			ScanIndexForward: aws.Bool(false),
		}

		if filter.Status != nil {
			input.FilterExpression = aws.String("#status = :status")
			input.ExpressionAttributeNames = map[string]string{"#status": "status"}
			input.ExpressionAttributeValues[":status"] = &ddbtypes.AttributeValueMemberS{Value: string(*filter.Status)}
		}

		result, err := s.client.Query(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("querying contracts: %w", err)
		}
		items = result.Items

		if result.LastEvaluatedKey != nil {
			if v, ok := result.LastEvaluatedKey["contract_id"].(*ddbtypes.AttributeValueMemberS); ok {
				nextCursor = v.Value
			}
		}
	}

	contracts := make([]*SignedConnectionContract, 0, len(items))
	for _, item := range items {
		var ci contractItem
		if err := attributevalue.UnmarshalMap(item, &ci); err != nil {
			return nil, fmt.Errorf("unmarshaling contract: %w", err)
		}
		contracts = append(contracts, ci.Data)
	}

	return &ContractListResult{
		Contracts:  contracts,
		NextCursor: nextCursor,
	}, nil
}

// DeleteContract permanently removes a contract.
func (s *DynamoDBStore) DeleteContract(ctx context.Context, contractID string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]ddbtypes.AttributeValue{
			"service_id":  &ddbtypes.AttributeValueMemberS{Value: s.serviceID},
			"contract_id": &ddbtypes.AttributeValueMemberS{Value: contractID},
		},
		ConditionExpression: aws.String("attribute_exists(contract_id)"),
	})

	if err != nil {
		var ccf *ddbtypes.ConditionalCheckFailedException
		if stderrors.As(err, &ccf) {
			return ErrContractNotFound
		}
		return fmt.Errorf("deleting contract: %w", err)
	}

	return nil
}

// inviteItem is the DynamoDB item representation of an invite.
type inviteItem struct {
	ServiceID string          `dynamodbav:"service_id"`
	InviteID  string          `dynamodbav:"contract_id"` // Use contract_id slot for invites
	Data      *ContractInvite `dynamodbav:"data"`
	TTL       int64           `dynamodbav:"ttl"`
}

// SaveInvite stores a new contract invite.
func (s *DynamoDBStore) SaveInvite(ctx context.Context, invite *ContractInvite) error {
	item := inviteItem{
		ServiceID: s.serviceID,
		InviteID:  "invite#" + invite.InviteID,
		Data:      invite,
		TTL:       invite.ExpiresAt.Unix(),
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("marshaling invite: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      av,
	})

	return err
}

// GetInvite retrieves an invite by ID.
func (s *DynamoDBStore) GetInvite(ctx context.Context, inviteID string) (*ContractInvite, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]ddbtypes.AttributeValue{
			"service_id":  &ddbtypes.AttributeValueMemberS{Value: s.serviceID},
			"contract_id": &ddbtypes.AttributeValueMemberS{Value: "invite#" + inviteID},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("getting invite: %w", err)
	}

	if result.Item == nil {
		return nil, ErrInviteNotFound
	}

	var item inviteItem
	if err := attributevalue.UnmarshalMap(result.Item, &item); err != nil {
		return nil, fmt.Errorf("unmarshaling invite: %w", err)
	}

	if time.Now().After(item.Data.ExpiresAt) {
		return nil, ErrInviteExpired
	}

	return item.Data, nil
}

// UseInvite increments the use count of an invite.
func (s *DynamoDBStore) UseInvite(ctx context.Context, inviteID string) error {
	invite, err := s.GetInvite(ctx, inviteID)
	if err != nil {
		return err
	}

	if invite.Uses >= invite.MaxUses {
		return ErrInviteExhausted
	}

	invite.Uses++
	return s.SaveInvite(ctx, invite)
}

// DeleteInvite removes an invite.
func (s *DynamoDBStore) DeleteInvite(ctx context.Context, inviteID string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]ddbtypes.AttributeValue{
			"service_id":  &ddbtypes.AttributeValueMemberS{Value: s.serviceID},
			"contract_id": &ddbtypes.AttributeValueMemberS{Value: "invite#" + inviteID},
		},
	})
	return err
}

// CleanupExpired removes expired contracts and invites.
// Note: For production, use DynamoDB TTL feature instead.
func (s *DynamoDBStore) CleanupExpired(ctx context.Context) (int, error) {
	// DynamoDB TTL handles this automatically
	// This is a no-op when TTL is configured
	return 0, nil
}

// amendmentItem is the DynamoDB item representation of an amendment.
type amendmentItem struct {
	ServiceID   string             `dynamodbav:"service_id"`
	AmendmentID string             `dynamodbav:"contract_id"` // Use contract_id slot with prefix
	ContractID  string             `dynamodbav:"amendment_contract_id"`
	Status      string             `dynamodbav:"status"`
	Data        *ContractAmendment `dynamodbav:"data"`
	TTL         int64              `dynamodbav:"ttl,omitempty"`
}

// SaveAmendment stores a new contract amendment.
func (s *DynamoDBStore) SaveAmendment(ctx context.Context, amendment *ContractAmendment) error {
	item := amendmentItem{
		ServiceID:   s.serviceID,
		AmendmentID: "amendment#" + amendment.AmendmentID,
		ContractID:  amendment.ContractID,
		Status:      string(amendment.Status),
		Data:        amendment,
		TTL:         amendment.ExpiresAt.Unix(),
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("marshaling amendment: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(contract_id)"),
	})

	if err != nil {
		var ccf *ddbtypes.ConditionalCheckFailedException
		if stderrors.As(err, &ccf) {
			return ErrAmendmentExists
		}
		return fmt.Errorf("saving amendment: %w", err)
	}

	return nil
}

// GetAmendment retrieves an amendment by ID.
func (s *DynamoDBStore) GetAmendment(ctx context.Context, amendmentID string) (*ContractAmendment, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]ddbtypes.AttributeValue{
			"service_id":  &ddbtypes.AttributeValueMemberS{Value: s.serviceID},
			"contract_id": &ddbtypes.AttributeValueMemberS{Value: "amendment#" + amendmentID},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("getting amendment: %w", err)
	}

	if result.Item == nil {
		return nil, ErrAmendmentNotFound
	}

	var item amendmentItem
	if err := attributevalue.UnmarshalMap(result.Item, &item); err != nil {
		return nil, fmt.Errorf("unmarshaling amendment: %w", err)
	}

	return item.Data, nil
}

// UpdateAmendment updates an existing amendment.
func (s *DynamoDBStore) UpdateAmendment(ctx context.Context, amendment *ContractAmendment) error {
	item := amendmentItem{
		ServiceID:   s.serviceID,
		AmendmentID: "amendment#" + amendment.AmendmentID,
		ContractID:  amendment.ContractID,
		Status:      string(amendment.Status),
		Data:        amendment,
	}

	// Clear TTL for completed amendments
	if amendment.Status == types.AmendmentStatusApplied ||
		amendment.Status == types.AmendmentStatusRejected {
		item.TTL = 0
	} else {
		item.TTL = amendment.ExpiresAt.Unix()
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("marshaling amendment: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_exists(contract_id)"),
	})

	if err != nil {
		var ccf *ddbtypes.ConditionalCheckFailedException
		if stderrors.As(err, &ccf) {
			return ErrAmendmentNotFound
		}
		return fmt.Errorf("updating amendment: %w", err)
	}

	return nil
}

// ListAmendments retrieves amendments matching the filter.
func (s *DynamoDBStore) ListAmendments(ctx context.Context, filter AmendmentFilter) ([]*ContractAmendment, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	// Query by service_id with amendment# prefix
	input := &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		KeyConditionExpression: aws.String("service_id = :sid AND begins_with(contract_id, :prefix)"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":sid":    &ddbtypes.AttributeValueMemberS{Value: s.serviceID},
			":prefix": &ddbtypes.AttributeValueMemberS{Value: "amendment#"},
		},
		Limit: aws.Int32(int32(limit)),
	}

	// Add filters
	var filterExprs []string
	if filter.ContractID != "" {
		filterExprs = append(filterExprs, "amendment_contract_id = :cid")
		input.ExpressionAttributeValues[":cid"] = &ddbtypes.AttributeValueMemberS{Value: filter.ContractID}
	}
	if filter.Status != nil {
		filterExprs = append(filterExprs, "#status = :status")
		if input.ExpressionAttributeNames == nil {
			input.ExpressionAttributeNames = make(map[string]string)
		}
		input.ExpressionAttributeNames["#status"] = "status"
		input.ExpressionAttributeValues[":status"] = &ddbtypes.AttributeValueMemberS{Value: string(*filter.Status)}
	}

	if len(filterExprs) > 0 {
		expr := filterExprs[0]
		for i := 1; i < len(filterExprs); i++ {
			expr += " AND " + filterExprs[i]
		}
		input.FilterExpression = aws.String(expr)
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("querying amendments: %w", err)
	}

	amendments := make([]*ContractAmendment, 0, len(result.Items))
	for _, item := range result.Items {
		var ai amendmentItem
		if err := attributevalue.UnmarshalMap(item, &ai); err != nil {
			return nil, fmt.Errorf("unmarshaling amendment: %w", err)
		}
		amendments = append(amendments, ai.Data)
	}

	return amendments, nil
}

// DeleteAmendment removes an amendment.
func (s *DynamoDBStore) DeleteAmendment(ctx context.Context, amendmentID string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]ddbtypes.AttributeValue{
			"service_id":  &ddbtypes.AttributeValueMemberS{Value: s.serviceID},
			"contract_id": &ddbtypes.AttributeValueMemberS{Value: "amendment#" + amendmentID},
		},
	})
	return err
}

