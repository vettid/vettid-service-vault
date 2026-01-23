// Support Center Example
//
// This example demonstrates how to use the VettID Service Vault to implement
// a customer support center with voice/video call capabilities.
//
// Features demonstrated:
// - Initiating calls to users (voice and video)
// - Handling call acceptance/rejection
// - WebRTC signaling flow
// - Webhook callbacks for call events
//
// In production, this would integrate with your WebRTC media server (e.g., Janus, mediasoup).
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// SupportCenter manages support agents and their calls
type SupportCenter struct {
	mu           sync.RWMutex
	vaultBaseURL string
	apiKey       string
	agents       map[string]*Agent
	activeCalls  map[string]*SupportCall
	callQueue    chan *SupportCall
}

// Agent represents a support agent
type Agent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"` // available, busy, offline
	CurrentCall string `json:"current_call,omitempty"`
}

// SupportCall represents an active or pending support call
type SupportCall struct {
	CallID      string                 `json:"call_id"`
	RequestID   string                 `json:"request_id"`
	UserID      string                 `json:"user_id"`
	AgentID     string                 `json:"agent_id,omitempty"`
	Type        string                 `json:"type"` // voice, video
	Status      string                 `json:"status"`
	Purpose     string                 `json:"purpose,omitempty"`
	StartedAt   time.Time              `json:"started_at"`
	ConnectedAt *time.Time             `json:"connected_at,omitempty"`
	EndedAt     *time.Time             `json:"ended_at,omitempty"`
	WebRTCData  map[string]interface{} `json:"webrtc_data,omitempty"`
}

// CallRequest is the API request to initiate a call
type CallRequest struct {
	UserID  string `json:"user_id"`
	Type    string `json:"type"` // voice, video
	Purpose string `json:"purpose,omitempty"`
	AgentID string `json:"agent_id,omitempty"` // Optional specific agent
}

// WebhookPayload represents incoming webhook data
type WebhookPayload struct {
	EventType string                 `json:"event_type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

func NewSupportCenter(vaultURL, apiKey string) *SupportCenter {
	return &SupportCenter{
		vaultBaseURL: vaultURL,
		apiKey:       apiKey,
		agents:       make(map[string]*Agent),
		activeCalls:  make(map[string]*SupportCall),
		callQueue:    make(chan *SupportCall, 100),
	}
}

func (sc *SupportCenter) AddAgent(id, name string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.agents[id] = &Agent{
		ID:     id,
		Name:   name,
		Status: "available",
	}
}

// InitiateCall starts a support call to a user
func (sc *SupportCenter) InitiateCall(userID, callType, purpose, agentID string) (*SupportCall, error) {
	// Find an available agent if not specified
	if agentID == "" {
		sc.mu.RLock()
		for _, agent := range sc.agents {
			if agent.Status == "available" {
				agentID = agent.ID
				break
			}
		}
		sc.mu.RUnlock()
	}

	if agentID == "" {
		return nil, fmt.Errorf("no agents available")
	}

	// Call the VettID Service Vault API to initiate the call
	// In production, this would be an HTTP POST to the vault
	// POST /api/v1/call/initiate

	// Mock response for example
	call := &SupportCall{
		RequestID: fmt.Sprintf("req_%d", time.Now().UnixNano()),
		UserID:    userID,
		AgentID:   agentID,
		Type:      callType,
		Status:    "pending",
		Purpose:   purpose,
		StartedAt: time.Now(),
	}

	// Store the call
	sc.mu.Lock()
	sc.activeCalls[call.RequestID] = call
	if agent, ok := sc.agents[agentID]; ok {
		agent.Status = "busy"
		agent.CurrentCall = call.RequestID
	}
	sc.mu.Unlock()

	log.Printf("Initiated %s call to user %s (request: %s, agent: %s)", callType, userID, call.RequestID, agentID)
	return call, nil
}

// HandleCallWebhook processes webhook events from the vault
func (sc *SupportCenter) HandleCallWebhook(w http.ResponseWriter, r *http.Request) {
	var payload WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	log.Printf("Received webhook: %s", payload.EventType)

	switch payload.EventType {
	case "call.accepted":
		sc.handleCallAccepted(payload.Data)
	case "call.rejected":
		sc.handleCallRejected(payload.Data)
	case "call.missed":
		sc.handleCallMissed(payload.Data)
	case "call.ended":
		sc.handleCallEnded(payload.Data)
	default:
		log.Printf("Unknown event type: %s", payload.EventType)
	}

	w.WriteHeader(http.StatusOK)
}

func (sc *SupportCenter) handleCallAccepted(data map[string]interface{}) {
	requestID, _ := data["request_id"].(string)
	callID, _ := data["call_id"].(string)

	sc.mu.Lock()
	defer sc.mu.Unlock()

	if call, ok := sc.activeCalls[requestID]; ok {
		call.CallID = callID
		call.Status = "connected"
		now := time.Now()
		call.ConnectedAt = &now

		// In production, you would now:
		// 1. Set up the WebRTC connection with the agent's browser
		// 2. Exchange SDP answer from agent to user
		// 3. Begin ICE candidate exchange

		log.Printf("Call %s connected with user %s", callID, call.UserID)
	}
}

func (sc *SupportCenter) handleCallRejected(data map[string]interface{}) {
	requestID, _ := data["request_id"].(string)
	reason, _ := data["reason"].(string)

	sc.mu.Lock()
	defer sc.mu.Unlock()

	if call, ok := sc.activeCalls[requestID]; ok {
		call.Status = "rejected"
		now := time.Now()
		call.EndedAt = &now

		// Free up the agent
		if agent, ok := sc.agents[call.AgentID]; ok {
			agent.Status = "available"
			agent.CurrentCall = ""
		}

		log.Printf("Call %s rejected: %s", requestID, reason)
		delete(sc.activeCalls, requestID)
	}
}

func (sc *SupportCenter) handleCallMissed(data map[string]interface{}) {
	requestID, _ := data["request_id"].(string)

	sc.mu.Lock()
	defer sc.mu.Unlock()

	if call, ok := sc.activeCalls[requestID]; ok {
		call.Status = "missed"
		now := time.Now()
		call.EndedAt = &now

		// Free up the agent
		if agent, ok := sc.agents[call.AgentID]; ok {
			agent.Status = "available"
			agent.CurrentCall = ""
		}

		log.Printf("Call %s missed (user didn't answer)", requestID)
		delete(sc.activeCalls, requestID)
	}
}

func (sc *SupportCenter) handleCallEnded(data map[string]interface{}) {
	callID, _ := data["call_id"].(string)

	sc.mu.Lock()
	defer sc.mu.Unlock()

	for requestID, call := range sc.activeCalls {
		if call.CallID == callID {
			call.Status = "ended"
			now := time.Now()
			call.EndedAt = &now

			// Free up the agent
			if agent, ok := sc.agents[call.AgentID]; ok {
				agent.Status = "available"
				agent.CurrentCall = ""
			}

			log.Printf("Call %s ended", callID)
			delete(sc.activeCalls, requestID)
			break
		}
	}
}

// HTTP Handlers

func (sc *SupportCenter) handleInitiateCall(w http.ResponseWriter, r *http.Request) {
	var req CallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.UserID == "" {
		http.Error(w, "user_id required", http.StatusBadRequest)
		return
	}

	if req.Type == "" {
		req.Type = "voice" // Default to voice call
	}

	call, err := sc.InitiateCall(req.UserID, req.Type, req.Purpose, req.AgentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(call)
}

func (sc *SupportCenter) handleGetAgents(w http.ResponseWriter, r *http.Request) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	agents := make([]*Agent, 0, len(sc.agents))
	for _, agent := range sc.agents {
		agents = append(agents, agent)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

func (sc *SupportCenter) handleGetCalls(w http.ResponseWriter, r *http.Request) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	calls := make([]*SupportCall, 0, len(sc.activeCalls))
	for _, call := range sc.activeCalls {
		calls = append(calls, call)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(calls)
}

func main() {
	// Initialize support center
	// In production, these would come from environment variables
	sc := NewSupportCenter(
		"http://localhost:8080", // VettID Service Vault URL
		"your-api-key",          // API key
	)

	// Add some agents
	sc.AddAgent("agent-001", "Alice Support")
	sc.AddAgent("agent-002", "Bob Support")
	sc.AddAgent("agent-003", "Carol Support")

	// Set up HTTP router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// API routes
	r.Post("/api/calls", sc.handleInitiateCall)
	r.Get("/api/agents", sc.handleGetAgents)
	r.Get("/api/calls", sc.handleGetCalls)

	// Webhook endpoint for VettID callbacks
	r.Post("/webhooks/vettid/call", sc.HandleCallWebhook)

	// Simple health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	log.Println("Support Center starting on :3000")
	log.Println("")
	log.Println("Endpoints:")
	log.Println("  POST /api/calls        - Initiate a call to a user")
	log.Println("  GET  /api/agents       - List support agents")
	log.Println("  GET  /api/calls        - List active calls")
	log.Println("  POST /webhooks/vettid/call - VettID webhook endpoint")
	log.Println("")
	log.Println("Example: curl -X POST http://localhost:3000/api/calls \\")
	log.Println("  -H 'Content-Type: application/json' \\")
	log.Println("  -d '{\"user_id\": \"user123\", \"type\": \"video\", \"purpose\": \"Technical support\"}'")

	if err := http.ListenAndServe(":3000", r); err != nil {
		log.Fatal(err)
	}
}
