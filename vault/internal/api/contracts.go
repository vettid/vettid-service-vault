package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/vettid/vettid-service-vault/vault/internal/contract"
	"github.com/vettid/vettid-service-vault/vault/pkg/types"
)

// ContractListInput is the query parameters for listing contracts.
type ContractListInput struct {
	UserID string `json:"user_id,omitempty"`
	Status string `json:"status,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// ContractListOutput is the output from listing contracts.
type ContractListOutput struct {
	Contracts  []ContractSummary `json:"contracts"`
	NextCursor string            `json:"next_cursor,omitempty"`
	TotalCount int               `json:"total_count,omitempty"`
}

// ContractSummary is a summary of a contract.
type ContractSummary struct {
	ContractID   string               `json:"contract_id"`
	UserID       string               `json:"user_id"`
	OfferingName string               `json:"offering_name"`
	Status       types.ContractStatus `json:"status"`
	CreatedAt    string               `json:"created_at"`
	ActivatedAt  string               `json:"activated_at,omitempty"`
}

// ContractDetail is the full detail of a contract.
type ContractDetail struct {
	ContractID   string               `json:"contract_id"`
	UserID       string               `json:"user_id"`
	ServiceID    string               `json:"service_id"`
	OfferingID   string               `json:"offering_id"`
	OfferingName string               `json:"offering_name"`
	Status       types.ContractStatus `json:"status"`
	Capabilities []string             `json:"capabilities"`
	CreatedAt    string               `json:"created_at"`
	ActivatedAt  string               `json:"activated_at,omitempty"`
	ExpiresAt    string               `json:"expires_at,omitempty"`
}

// InviteInput is the input for creating an invite.
type InviteInput struct {
	OfferingID string            `json:"offering_id"`
	MaxUses    int               `json:"max_uses,omitempty"`
	TTL        string            `json:"ttl,omitempty"` // Duration string
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// InviteOutput is the output from creating an invite.
type InviteOutput struct {
	InviteID   string `json:"invite_id"`
	OfferingID string `json:"offering_id"`
	ExpiresAt  string `json:"expires_at"`
	MaxUses    int    `json:"max_uses"`
	InviteURL  string `json:"invite_url,omitempty"`
}

// listContracts lists contracts for this service.
// GET /api/v1/contracts
func (s *Server) listContracts(w http.ResponseWriter, r *http.Request) {
	if s.contracts == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "contract store not configured")
		return
	}

	// Parse query parameters
	q := r.URL.Query()

	var status *types.ContractStatus
	if s := q.Get("status"); s != "" {
		cs := types.ContractStatus(s)
		status = &cs
	}

	limit := 50
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	filter := contract.ContractFilter{
		UserID: q.Get("user_id"),
		Status: status,
		Limit:  limit,
		Cursor: q.Get("cursor"),
	}

	result, err := s.contracts.ListContracts(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	// Convert to summaries
	summaries := make([]ContractSummary, 0, len(result.Contracts))
	for _, c := range result.Contracts {
		summary := ContractSummary{
			ContractID:   c.ContractID,
			UserID:       c.UserID,
			OfferingName: c.OfferingSnapshot.Name,
			Status:       c.Status,
			CreatedAt:    c.CreatedAt.Format(time.RFC3339),
		}
		if c.ActivatedAt != nil {
			summary.ActivatedAt = c.ActivatedAt.Format(time.RFC3339)
		}
		summaries = append(summaries, summary)
	}

	output := ContractListOutput{
		Contracts:  summaries,
		NextCursor: result.NextCursor,
		TotalCount: result.TotalCount,
	}

	writeJSON(w, http.StatusOK, output)
}

// getContract gets a specific contract.
// GET /api/v1/contracts/{contractID}
func (s *Server) getContract(w http.ResponseWriter, r *http.Request) {
	if s.contracts == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "contract store not configured")
		return
	}

	contractID := chi.URLParam(r, "contractID")
	if contractID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "contract ID is required")
		return
	}

	c, err := s.contracts.GetContract(r.Context(), contractID)
	if err != nil {
		if err == contract.ErrContractNotFound {
			writeError(w, http.StatusNotFound, types.ErrCodeNotFound, "contract not found")
			return
		}
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	// Extract capability names
	caps := make([]string, 0, len(c.OfferingSnapshot.Capabilities))
	for _, cap := range c.OfferingSnapshot.Capabilities {
		caps = append(caps, string(cap.Capability))
	}

	detail := ContractDetail{
		ContractID:   c.ContractID,
		UserID:       c.UserID,
		ServiceID:    c.ServiceID,
		OfferingID:   c.OfferingID,
		OfferingName: c.OfferingSnapshot.Name,
		Status:       c.Status,
		Capabilities: caps,
		CreatedAt:    c.CreatedAt.Format(time.RFC3339),
	}

	if c.ActivatedAt != nil {
		detail.ActivatedAt = c.ActivatedAt.Format(time.RFC3339)
	}
	if c.ExpiresAt != nil {
		detail.ExpiresAt = c.ExpiresAt.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, detail)
}

// generateInvite creates a new contract invite.
// POST /api/v1/contracts/invite
func (s *Server) generateInvite(w http.ResponseWriter, r *http.Request) {
	if s.negotiator == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "contract negotiator not configured")
		return
	}

	var input InviteInput
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, err.Error())
		return
	}

	if input.OfferingID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "offering_id is required")
		return
	}

	maxUses := input.MaxUses
	if maxUses <= 0 {
		maxUses = 1
	}

	ttl := 24 * time.Hour
	if input.TTL != "" {
		if d, err := time.ParseDuration(input.TTL); err == nil && d > 0 {
			ttl = d
		}
	}

	invite, err := s.negotiator.CreateInvite(r.Context(), input.OfferingID, maxUses, ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	output := InviteOutput{
		InviteID:   invite.InviteID,
		OfferingID: invite.OfferingID,
		ExpiresAt:  invite.ExpiresAt.Format(time.RFC3339),
		MaxUses:    invite.MaxUses,
	}

	// Build invite URL if we have a domain
	if s.identity != nil && s.identity.Domain != "" {
		output.InviteURL = "https://" + s.identity.Domain + "/connect/" + invite.InviteID
	}

	writeJSON(w, http.StatusCreated, output)
}

// cancelContract cancels an active contract.
// DELETE /api/v1/contracts/{contractID}
func (s *Server) cancelContract(w http.ResponseWriter, r *http.Request) {
	if s.negotiator == nil {
		writeError(w, http.StatusServiceUnavailable, types.ErrCodeInternal, "contract negotiator not configured")
		return
	}

	contractID := chi.URLParam(r, "contractID")
	if contractID == "" {
		writeError(w, http.StatusBadRequest, types.ErrCodeInvalidRequest, "contract ID is required")
		return
	}

	reason := r.URL.Query().Get("reason")
	if reason == "" {
		reason = "cancelled by service"
	}

	if err := s.negotiator.CancelContract(r.Context(), contractID, reason); err != nil {
		if err == contract.ErrContractNotFound {
			writeError(w, http.StatusNotFound, types.ErrCodeNotFound, "contract not found")
			return
		}
		writeError(w, http.StatusInternalServerError, types.ErrCodeInternal, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "cancelled",
		"message": "contract has been cancelled",
	})
}
