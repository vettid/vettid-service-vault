// Example: VettID Auth Service
//
// This example demonstrates integrating VettID Service Vault
// for passwordless authentication in a web application.
//
// NOTE: This is a simplified example. In production:
// - Use proper HTML templating (not inline HTML)
// - Use textContent instead of innerHTML where possible
// - Implement proper CSRF protection
// - Use secure session storage (Redis, database)
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Configuration from environment
type Config struct {
	Port          string
	BaseURL       string
	VaultEndpoint string
	VaultAPIKey   string
	WebhookSecret string
}

func loadConfig() *Config {
	return &Config{
		Port:          getEnv("PORT", "3000"),
		BaseURL:       getEnv("BASE_URL", "http://localhost:3000"),
		VaultEndpoint: getEnv("VAULT_ENDPOINT", "http://localhost:8080"),
		VaultAPIKey:   getEnv("VAULT_API_KEY", ""),
		WebhookSecret: getEnv("WEBHOOK_SECRET", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// VaultClient wraps the Service Vault API
type VaultClient struct {
	endpoint      string
	apiKey        string
	webhookSecret string
	httpClient    *http.Client
}

func NewVaultClient(endpoint, apiKey, webhookSecret string) *VaultClient {
	return &VaultClient{
		endpoint:      endpoint,
		apiKey:        apiKey,
		webhookSecret: webhookSecret,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
	}
}

// AuthRequest represents an authentication request to the vault
type AuthRequest struct {
	UserID            string                 `json:"user_id,omitempty"`
	Purpose           string                 `json:"purpose"`
	Context           map[string]interface{} `json:"context,omitempty"`
	TimeoutSeconds    int                    `json:"timeout_seconds"`
	OfflineGraceHours int                    `json:"offline_grace_hours"`
	CallbackURL       string                 `json:"callback_url"`
}

// AuthRequestResponse is the response from requesting auth
type AuthRequestResponse struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at"`
	QRURL     string `json:"qr_url,omitempty"`
}

// RequestAuth sends an authentication request to the vault
func (c *VaultClient) RequestAuth(ctx context.Context, req *AuthRequest) (*AuthRequestResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/api/v1/auth/request", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault error: %s", string(body))
	}

	var result AuthRequestResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// VerifySignature verifies a webhook signature
func (c *VaultClient) VerifySignature(payload []byte, signature string) bool {
	if c.webhookSecret == "" {
		return true // Skip verification if no secret configured
	}

	h := hmac.New(sha256.New, []byte(c.webhookSecret))
	h.Write(payload)
	expected := "sha256=" + hex.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

// SessionStore manages user sessions (in-memory for this example)
type SessionStore struct {
	mu             sync.RWMutex
	sessions       map[string]*Session
	pendingLogins  map[string]*PendingLogin
	requestToLogin map[string]string // requestID -> loginID
}

type Session struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type PendingLogin struct {
	ID        string
	RequestID string
	Status    string // pending, completed, failed, expired
	SessionID string
	Error     string
	CreatedAt time.Time
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions:       make(map[string]*Session),
		pendingLogins:  make(map[string]*PendingLogin),
		requestToLogin: make(map[string]string),
	}
}

func (s *SessionStore) CreatePendingLogin(loginID, requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pendingLogins[loginID] = &PendingLogin{
		ID:        loginID,
		RequestID: requestID,
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	s.requestToLogin[requestID] = loginID
}

func (s *SessionStore) GetPendingLogin(loginID string) (*PendingLogin, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	login, ok := s.pendingLogins[loginID]
	return login, ok
}

func (s *SessionStore) CompleteLogin(requestID, userID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	loginID, ok := s.requestToLogin[requestID]
	if !ok {
		return ""
	}

	login, ok := s.pendingLogins[loginID]
	if !ok {
		return ""
	}

	// Create session
	sessionID := generateID()
	s.sessions[sessionID] = &Session{
		ID:        sessionID,
		UserID:    userID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	// Update login
	login.Status = "completed"
	login.SessionID = sessionID

	return sessionID
}

func (s *SessionStore) FailLogin(requestID, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	loginID, ok := s.requestToLogin[requestID]
	if !ok {
		return
	}

	login, ok := s.pendingLogins[loginID]
	if !ok {
		return
	}

	login.Status = "failed"
	login.Error = reason
}

func (s *SessionStore) GetSession(sessionID string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[sessionID]
	if !ok || time.Now().After(session.ExpiresAt) {
		return nil, false
	}
	return session, true
}

func (s *SessionStore) DeleteSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

// Handlers

type Handlers struct {
	vault   *VaultClient
	store   *SessionStore
	baseURL string
}

func NewHandlers(vault *VaultClient, store *SessionStore, baseURL string) *Handlers {
	return &Handlers{
		vault:   vault,
		store:   store,
		baseURL: baseURL,
	}
}

// HomePage serves the home page
func (h *Handlers) HomePage(w http.ResponseWriter, r *http.Request) {
	session := h.getSessionFromRequest(r)
	w.Header().Set("Content-Type", "text/html")

	if session != nil {
		// User is logged in - use template with escaped values
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>VettID Auth Example</title>
    <style>
        body { font-family: system-ui; max-width: 600px; margin: 50px auto; padding: 20px; }
        .btn { padding: 10px 20px; background: #0066cc; color: white; border: none; border-radius: 5px; cursor: pointer; }
        .btn:hover { background: #0052a3; }
        .user-info { background: #f0f0f0; padding: 20px; border-radius: 5px; }
    </style>
</head>
<body>
    <h1>VettID Auth Example</h1>
    <div class="user-info">
        <p><strong>Logged in as:</strong> <span id="user-id"></span></p>
        <p><strong>Session expires:</strong> <span id="expires"></span></p>
        <form action="/logout" method="POST">
            <button type="submit" class="btn">Logout</button>
        </form>
    </div>
    <script>
        // Safely set text content (no HTML interpretation)
        document.getElementById('user-id').textContent = %q;
        document.getElementById('expires').textContent = %q;
    </script>
</body>
</html>`, session.UserID, session.ExpiresAt.Format(time.RFC3339))
	} else {
		// User is not logged in
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <title>VettID Auth Example</title>
    <style>
        body { font-family: system-ui; max-width: 600px; margin: 50px auto; padding: 20px; }
        .btn { padding: 10px 20px; background: #0066cc; color: white; border: none; border-radius: 5px; cursor: pointer; text-decoration: none; display: inline-block; }
        .btn:hover { background: #0052a3; }
    </style>
</head>
<body>
    <h1>VettID Auth Example</h1>
    <p>Welcome! Click below to login with VettID.</p>
    <a href="/login" class="btn">Login with VettID</a>
</body>
</html>`)
	}
}

// LoginPage serves the login page with QR code
func (h *Handlers) LoginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
    <title>Login with VettID</title>
    <style>
        body { font-family: system-ui; max-width: 600px; margin: 50px auto; padding: 20px; text-align: center; }
        #qr-container { margin: 30px 0; }
        #status { padding: 10px; margin: 20px 0; border-radius: 5px; }
        .pending { background: #fff3cd; }
        .success { background: #d4edda; }
        .error { background: #f8d7da; }
    </style>
</head>
<body>
    <h1>Login with VettID</h1>
    <p>Scan the QR code with your VettID app to login.</p>

    <div id="qr-container">
        <p>Loading...</p>
    </div>

    <div id="status" class="pending">
        Waiting for approval...
    </div>

    <script>
        let loginId = null;

        async function startLogin() {
            try {
                const resp = await fetch('/api/login/start', { method: 'POST' });
                const data = await resp.json();
                loginId = data.login_id;

                // Safely display login ID using textContent (no HTML interpretation)
                const container = document.getElementById('qr-container');
                container.textContent = '';

                const p1 = document.createElement('p');
                const strong = document.createElement('strong');
                strong.textContent = 'Login ID: ';
                p1.appendChild(strong);
                p1.appendChild(document.createTextNode(loginId));
                container.appendChild(p1);

                const p2 = document.createElement('p');
                const small = document.createElement('small');
                small.textContent = 'In production, this would be a QR code';
                p2.appendChild(small);
                container.appendChild(p2);

                // Start polling
                pollStatus();
            } catch (err) {
                const status = document.getElementById('status');
                status.className = 'error';
                status.textContent = 'Error: ' + err.message;
            }
        }

        async function pollStatus() {
            if (!loginId) return;

            try {
                const resp = await fetch('/api/login/status/' + encodeURIComponent(loginId));
                const data = await resp.json();
                const status = document.getElementById('status');

                if (data.status === 'completed') {
                    status.className = 'success';
                    status.textContent = 'Login successful! Redirecting...';
                    // Set session cookie and redirect
                    document.cookie = 'session=' + encodeURIComponent(data.session_id) + '; path=/';
                    setTimeout(function() { window.location.href = '/'; }, 1000);
                    return;
                }

                if (data.status === 'failed') {
                    status.className = 'error';
                    status.textContent = 'Login failed: ' + (data.error || 'User denied');
                    return;
                }

                // Still pending, poll again
                setTimeout(pollStatus, 2000);
            } catch (err) {
                setTimeout(pollStatus, 5000);
            }
        }

        startLogin();
    </script>
</body>
</html>`)
}

// StartLogin initiates the login flow
func (h *Handlers) StartLogin(w http.ResponseWriter, r *http.Request) {
	loginID := generateID()

	// Request auth from vault
	resp, err := h.vault.RequestAuth(r.Context(), &AuthRequest{
		Purpose: "Login to Example App",
		Context: map[string]interface{}{
			"login_id": loginID,
			"ip":       r.RemoteAddr,
		},
		TimeoutSeconds:    300, // 5 minutes
		OfflineGraceHours: 0,   // No offline approval for login
		CallbackURL:       h.baseURL + "/api/webhooks/vettid",
	})

	if err != nil {
		log.Printf("Error requesting auth: %v", err)
		http.Error(w, "Failed to start login", http.StatusInternalServerError)
		return
	}

	// Store pending login
	h.store.CreatePendingLogin(loginID, resp.RequestID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"login_id":   loginID,
		"request_id": resp.RequestID,
		"qr_url":     resp.QRURL,
	})
}

// LoginStatus returns the status of a login attempt
func (h *Handlers) LoginStatus(w http.ResponseWriter, r *http.Request) {
	loginID := chi.URLParam(r, "id")

	login, ok := h.store.GetPendingLogin(loginID)
	if !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"status": login.Status,
	}

	if login.Status == "completed" {
		response["session_id"] = login.SessionID
	}
	if login.Error != "" {
		response["error"] = login.Error
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// WebhookEvent represents a VettID webhook event
type WebhookEvent struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	UserID    string `json:"user_id"`
	Status    string `json:"status"`
}

// HandleWebhook processes VettID webhooks
func (h *Handlers) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Verify signature
	signature := r.Header.Get("X-VettID-Signature")
	if !h.vault.VerifySignature(body, signature) {
		log.Printf("Invalid webhook signature")
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("Received webhook: type=%s request_id=%s status=%s", event.Type, event.RequestID, event.Status)

	switch event.Type {
	case "auth.response":
		if event.Status == "approved" || event.Status == "offline_approved" {
			sessionID := h.store.CompleteLogin(event.RequestID, event.UserID)
			log.Printf("Login completed: user=%s session=%s", event.UserID, sessionID)
		} else {
			h.store.FailLogin(event.RequestID, event.Status)
			log.Printf("Login failed: request=%s status=%s", event.RequestID, event.Status)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// GetCurrentUser returns the current authenticated user
func (h *Handlers) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	session := h.getSessionFromRequest(r)
	if session == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":    session.UserID,
		"session_id": session.ID,
		"expires_at": session.ExpiresAt,
	})
}

// Logout terminates the current session
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		h.store.DeleteSession(cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handlers) getSessionFromRequest(r *http.Request) *Session {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil
	}

	session, ok := h.store.GetSession(cookie.Value)
	if !ok {
		return nil
	}

	return session
}

// Helper functions

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func main() {
	cfg := loadConfig()

	vault := NewVaultClient(cfg.VaultEndpoint, cfg.VaultAPIKey, cfg.WebhookSecret)
	store := NewSessionStore()
	handlers := NewHandlers(vault, store, cfg.BaseURL)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Pages
	r.Get("/", handlers.HomePage)
	r.Get("/login", handlers.LoginPage)
	r.Post("/logout", handlers.Logout)

	// API
	r.Route("/api", func(r chi.Router) {
		r.Post("/login/start", handlers.StartLogin)
		r.Get("/login/status/{id}", handlers.LoginStatus)
		r.Post("/webhooks/vettid", handlers.HandleWebhook)
		r.Get("/me", handlers.GetCurrentUser)
	})

	log.Printf("Starting auth service on :%s", cfg.Port)
	log.Printf("Vault endpoint: %s", cfg.VaultEndpoint)
	log.Printf("Base URL: %s", cfg.BaseURL)

	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}
