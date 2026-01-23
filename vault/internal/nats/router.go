package nats

import (
	"context"
	"fmt"
	"sync"
)

// Router dispatches incoming messages to registered handlers based on event type.
//
// The router enables modular message handling by allowing different handlers
// to be registered for different event types (e.g., "auth.response", "contract.signed").
type Router struct {
	mu            sync.RWMutex
	handlers      map[string]MessageHandler
	defaultHandler MessageHandler
	middleware    []Middleware
	serviceSpace  *ServiceSpaceClient
}

// Middleware processes messages before they reach handlers.
type Middleware func(next MessageHandler) MessageHandler

// NewRouter creates a new message router.
func NewRouter() *Router {
	return &Router{
		handlers: make(map[string]MessageHandler),
	}
}

// RegisterHandler registers a handler for a specific event type.
func (r *Router) RegisterHandler(eventType string, handler MessageHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[eventType] = handler
}

// SetDefaultHandler sets the handler for unrecognized event types.
func (r *Router) SetDefaultHandler(handler MessageHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultHandler = handler
}

// Use adds middleware to the router.
// Middleware is applied in the order it was added.
func (r *Router) Use(mw Middleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middleware = append(r.middleware, mw)
}

// Route handles an incoming message by dispatching to the appropriate handler.
func (r *Router) Route(ctx context.Context, msg *Message) error {
	r.mu.RLock()
	handler, exists := r.handlers[msg.EventType]
	defaultHandler := r.defaultHandler
	middleware := r.middleware
	r.mu.RUnlock()

	if !exists {
		if defaultHandler != nil {
			handler = defaultHandler
		} else {
			return fmt.Errorf("no handler for event type: %s", msg.EventType)
		}
	}

	// Apply middleware in reverse order (so first added runs first)
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler)
	}

	return handler(ctx, msg)
}

// SetServiceSpace associates a ServiceSpace client with this router.
func (r *Router) SetServiceSpace(client *ServiceSpaceClient) {
	r.serviceSpace = client
	client.SetRouter(r)
}

// Start begins routing messages from the ServiceSpace client.
func (r *Router) Start() error {
	if r.serviceSpace == nil {
		return fmt.Errorf("no ServiceSpace client configured")
	}

	return r.serviceSpace.SubscribeToAllUsers(r.Route)
}

// Handler returns the handler for a given event type.
func (r *Router) Handler(eventType string) (MessageHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[eventType]
	return h, ok
}

// EventTypes returns all registered event types.
func (r *Router) EventTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.handlers))
	for t := range r.handlers {
		types = append(types, t)
	}
	return types
}

// Common middleware implementations

// LoggingMiddleware logs incoming messages.
func LoggingMiddleware(logger func(string, ...interface{})) Middleware {
	return func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, msg *Message) error {
			logger("received message: event=%s user=%s", msg.EventType, msg.UserID)
			err := next(ctx, msg)
			if err != nil {
				logger("handler error: event=%s user=%s error=%v", msg.EventType, msg.UserID, err)
			}
			return err
		}
	}
}

// RecoveryMiddleware recovers from panics in handlers.
func RecoveryMiddleware(onPanic func(interface{})) Middleware {
	return func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, msg *Message) (err error) {
			defer func() {
				if r := recover(); r != nil {
					if onPanic != nil {
						onPanic(r)
					}
					err = fmt.Errorf("handler panic: %v", r)
				}
			}()
			return next(ctx, msg)
		}
	}
}

// TimeoutMiddleware adds a timeout to message handling.
func TimeoutMiddleware(timeout context.Context) Middleware {
	return func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, msg *Message) error {
			ctx, cancel := context.WithTimeout(ctx, 30*1e9) // 30 seconds
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- next(ctx, msg)
			}()

			select {
			case err := <-done:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}
