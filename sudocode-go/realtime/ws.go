package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"encore.dev/rlog"
	"github.com/coder/websocket"
)

var hub = NewHub()

// GetHub returns the package-level hub for broadcasting events from other services.
func GetHub() *Hub {
	return hub
}

// WS handles WebSocket connections for real-time event streaming.
//
//encore:api public raw path=/api/ws
func WS(w http.ResponseWriter, r *http.Request) {
	projectID := r.Header.Get("X-Project-ID")

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Allow all origins for local dev; tighten in production.
		InsecureSkipVerify: true,
	})
	if err != nil {
		rlog.Error("websocket accept failed", "err", err)
		return
	}
	defer conn.CloseNow()

	c := &client{
		send: make(chan []byte, 64),
	}

	// If project ID provided via header, auto-subscribe to all events for that project.
	if projectID != "" {
		c.addSubscription(Subscription{ProjectID: projectID})
	}

	hub.register <- c
	defer func() { hub.unregister <- c }()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Writer goroutine: sends messages and pings.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case msg, ok := <-c.send:
				if !ok {
					return
				}
				writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
				err := conn.Write(writeCtx, websocket.MessageText, msg)
				writeCancel()
				if err != nil {
					cancel()
					return
				}
			case <-ticker.C:
				pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
				err := conn.Ping(pingCtx)
				pingCancel()
				if err != nil {
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Reader loop: process client messages.
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			break
		}

		var msg ClientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			sendError(c, "invalid JSON")
			continue
		}

		switch msg.Type {
		case "subscribe":
			if msg.Subscription.ProjectID == "" {
				sendError(c, "subscription requires project_id")
				continue
			}
			c.addSubscription(msg.Subscription)
			sendJSON(c, ServerMessage{Type: "subscribed"})

		case "unsubscribe":
			c.removeSubscription(msg.Subscription)
			sendJSON(c, ServerMessage{Type: "unsubscribed"})

		case "ping":
			sendJSON(c, ServerMessage{Type: "pong"})

		default:
			sendError(c, "unknown message type: "+msg.Type)
		}
	}
}

func sendJSON(c *client, msg ServerMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
	}
}

func sendError(c *client, errMsg string) {
	sendJSON(c, ServerMessage{Type: "error", Error: errMsg})
}
