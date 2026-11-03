package realtime

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

type Hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[chan []byte]struct{})}
}

func (h *Hub) Broadcast(event string, data any) {
	msg, err := json.Marshal(fiber.Map{"event": event, "data": data})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (h *Hub) subscribe() chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

func (h *Hub) Handler() fiber.Handler {
	return websocket.New(func(c *websocket.Conn) {
		ch := h.subscribe()
		defer h.unsubscribe(ch)

		closed := make(chan struct{})
		go func() {
			defer close(closed)
			for {
				if _, _, err := c.ReadMessage(); err != nil {
					return
				}
			}
		}()

		ping := time.NewTicker(30 * time.Second)
		defer ping.Stop()

		for {
			select {
			case msg := <-ch:
				if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-ping.C:
				if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-closed:
				return
			}
		}
	})
}
