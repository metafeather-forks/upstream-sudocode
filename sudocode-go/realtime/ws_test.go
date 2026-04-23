package realtime

import (
	"testing"
	"time"
)

func TestSubscriptionMatches(t *testing.T) {
	tests := []struct {
		name  string
		sub   Subscription
		event Event
		want  bool
	}{
		{
			name:  "exact project match",
			sub:   Subscription{ProjectID: "proj-1"},
			event: Event{ProjectID: "proj-1", EntityType: EntitySpec, EntityID: "s-abc"},
			want:  true,
		},
		{
			name:  "project mismatch",
			sub:   Subscription{ProjectID: "proj-1"},
			event: Event{ProjectID: "proj-2", EntityType: EntitySpec, EntityID: "s-abc"},
			want:  false,
		},
		{
			name:  "filter by entity type",
			sub:   Subscription{ProjectID: "proj-1", EntityType: EntityIssue},
			event: Event{ProjectID: "proj-1", EntityType: EntitySpec, EntityID: "s-abc"},
			want:  false,
		},
		{
			name:  "entity type matches",
			sub:   Subscription{ProjectID: "proj-1", EntityType: EntityIssue},
			event: Event{ProjectID: "proj-1", EntityType: EntityIssue, EntityID: "i-123"},
			want:  true,
		},
		{
			name:  "filter by entity id",
			sub:   Subscription{ProjectID: "proj-1", EntityType: EntitySpec, EntityID: "s-abc"},
			event: Event{ProjectID: "proj-1", EntityType: EntitySpec, EntityID: "s-xyz"},
			want:  false,
		},
		{
			name:  "exact entity id match",
			sub:   Subscription{ProjectID: "proj-1", EntityType: EntitySpec, EntityID: "s-abc"},
			event: Event{ProjectID: "proj-1", EntityType: EntitySpec, EntityID: "s-abc"},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sub.matches(tt.event)
			if got != tt.want {
				t.Errorf("Subscription.matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func waitForCount(h *Hub, n int) {
	deadline := time.After(time.Second)
	for {
		if h.ClientCount() == n {
			return
		}
		select {
		case <-deadline:
			return
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestHubBroadcastToSubscribedClients(t *testing.T) {
	h := NewHub()

	c1 := &client{send: make(chan []byte, 16)}
	c1.addSubscription(Subscription{ProjectID: "proj-1"})

	c2 := &client{send: make(chan []byte, 16)}
	c2.addSubscription(Subscription{ProjectID: "proj-2"})

	h.register <- c1
	h.register <- c2
	waitForCount(h, 2)

	h.Broadcast(Event{
		ProjectID:  "proj-1",
		EntityType: EntitySpec,
		EntityID:   "s-abc",
		Action:     ActionCreated,
	})

	select {
	case msg := <-c1.send:
		if len(msg) == 0 {
			t.Fatal("expected non-empty message for c1")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for c1 message")
	}

	// Give hub time to process, then check c2 got nothing.
	time.Sleep(50 * time.Millisecond)
	select {
	case <-c2.send:
		t.Fatal("c2 should not have received a message")
	default:
	}

	h.unregister <- c1
	h.unregister <- c2
}

func TestHubBroadcastEntityTypeFilter(t *testing.T) {
	h := NewHub()

	c := &client{send: make(chan []byte, 16)}
	c.addSubscription(Subscription{ProjectID: "proj-1", EntityType: EntityIssue})

	h.register <- c
	waitForCount(h, 1)

	// Broadcast spec event -- should not match.
	h.Broadcast(Event{
		ProjectID:  "proj-1",
		EntityType: EntitySpec,
		EntityID:   "s-abc",
		Action:     ActionUpdated,
	})

	time.Sleep(50 * time.Millisecond)
	select {
	case <-c.send:
		t.Fatal("should not receive spec event when subscribed to issues only")
	default:
	}

	// Broadcast issue event -- should match.
	h.Broadcast(Event{
		ProjectID:  "proj-1",
		EntityType: EntityIssue,
		EntityID:   "i-123",
		Action:     ActionCreated,
	})

	select {
	case msg := <-c.send:
		if len(msg) == 0 {
			t.Fatal("expected non-empty message")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: should have received issue event")
	}

	h.unregister <- c
}

func TestClientAddRemoveSubscription(t *testing.T) {
	c := &client{send: make(chan []byte, 16)}

	sub := Subscription{ProjectID: "proj-1", EntityType: EntitySpec}
	c.addSubscription(sub)
	c.addSubscription(sub) // duplicate

	if len(c.subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(c.subscriptions))
	}

	c.removeSubscription(sub)
	if len(c.subscriptions) != 0 {
		t.Fatalf("expected 0 subscriptions, got %d", len(c.subscriptions))
	}

	c.removeSubscription(sub) // safe no-op
}

func TestHubClientCount(t *testing.T) {
	h := NewHub()

	if h.ClientCount() != 0 {
		t.Fatalf("expected 0 clients, got %d", h.ClientCount())
	}

	c := &client{send: make(chan []byte, 16)}
	h.register <- c
	waitForCount(h, 1)

	if h.ClientCount() != 1 {
		t.Fatalf("expected 1 client, got %d", h.ClientCount())
	}

	h.unregister <- c
	waitForCount(h, 0)

	if h.ClientCount() != 0 {
		t.Fatalf("expected 0 clients after unregister, got %d", h.ClientCount())
	}
}
