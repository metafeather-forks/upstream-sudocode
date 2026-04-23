package realtime

import (
	"encoding/json"
	"sync"
)

// EntityType represents the type of entity that changed.
type EntityType string

const (
	EntitySpec         EntityType = "spec"
	EntityIssue        EntityType = "issue"
	EntityRelationship EntityType = "relationship"
	EntityFeedback     EntityType = "feedback"
)

// EventAction represents what happened to an entity.
type EventAction string

const (
	ActionCreated EventAction = "created"
	ActionUpdated EventAction = "updated"
	ActionDeleted EventAction = "deleted"
)

// Event is broadcast to subscribed clients.
type Event struct {
	ProjectID  string      `json:"project_id"`
	EntityType EntityType  `json:"entity_type"`
	EntityID   string      `json:"entity_id"`
	Action     EventAction `json:"action"`
	Data       any         `json:"data,omitempty"`
}

// Subscription describes what a client wants to receive.
type Subscription struct {
	ProjectID  string     `json:"project_id"`
	EntityType EntityType `json:"entity_type,omitempty"` // empty = all types
	EntityID   string     `json:"entity_id,omitempty"`   // empty = all of that type
}

// matches returns true if this subscription should receive the given event.
func (s Subscription) matches(e Event) bool {
	if s.ProjectID != e.ProjectID {
		return false
	}
	if s.EntityType != "" && s.EntityType != e.EntityType {
		return false
	}
	if s.EntityID != "" && s.EntityID != e.EntityID {
		return false
	}
	return true
}

// ClientMessage is received from WebSocket clients.
type ClientMessage struct {
	Type         string       `json:"type"` // "subscribe", "unsubscribe", "ping"
	Subscription Subscription `json:"subscription,omitempty"`
}

// ServerMessage is sent to WebSocket clients.
type ServerMessage struct {
	Type  string `json:"type"` // "event", "pong", "error", "subscribed", "unsubscribed"
	Event *Event `json:"event,omitempty"`
	Error string `json:"error,omitempty"`
}

// client represents a connected WebSocket client.
type client struct {
	send          chan []byte
	subscriptions []Subscription
	mu            sync.Mutex
}

func (c *client) addSubscription(sub Subscription) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Avoid duplicates.
	for _, s := range c.subscriptions {
		if s == sub {
			return
		}
	}
	c.subscriptions = append(c.subscriptions, sub)
}

func (c *client) removeSubscription(sub Subscription) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, s := range c.subscriptions {
		if s == sub {
			c.subscriptions = append(c.subscriptions[:i], c.subscriptions[i+1:]...)
			return
		}
	}
}

func (c *client) matchesEvent(e Event) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.subscriptions {
		if s.matches(e) {
			return true
		}
	}
	return false
}

// Hub manages WebSocket clients and event broadcasting.
type Hub struct {
	mu         sync.RWMutex
	clients    map[*client]struct{}
	register   chan *client
	unregister chan *client
	broadcast  chan Event
}

// NewHub creates a new Hub and starts its run loop.
func NewHub() *Hub {
	h := &Hub{
		clients:    make(map[*client]struct{}),
		register:   make(chan *client),
		unregister: make(chan *client),
		broadcast:  make(chan Event, 256),
	}
	go h.run()
	return h
}

func (h *Hub) run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = struct{}{}
			h.mu.Unlock()

		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()

		case event := <-h.broadcast:
			msg, err := json.Marshal(ServerMessage{Type: "event", Event: &event})
			if err != nil {
				continue
			}
			h.mu.RLock()
			for c := range h.clients {
				if c.matchesEvent(event) {
					select {
					case c.send <- msg:
					default:
						// Client too slow; drop message.
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends an event to all matching subscribers.
func (h *Hub) Broadcast(e Event) {
	h.broadcast <- e
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
