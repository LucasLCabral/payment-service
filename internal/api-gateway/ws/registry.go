package ws

import (
	"context"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/LucasLCabral/payment-service/pkg/logger"
)

type connEntry struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

type Registry struct {
	mu    sync.RWMutex
	conns map[string]*connEntry
	log   logger.Logger
}

func NewRegistry(log logger.Logger) *Registry {
	return &Registry{
		conns: make(map[string]*connEntry),
		log:   log,
	}
}

func (r *Registry) Register(paymentID string, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if old, ok := r.conns[paymentID]; ok && old != nil && old.conn != conn {
		_ = old.conn.Close()
	}
	r.conns[paymentID] = &connEntry{conn: conn}
}

func (r *Registry) RemoveIf(paymentID string, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if e, ok := r.conns[paymentID]; ok && e != nil && e.conn == conn {
		delete(r.conns, paymentID)
	}
}

func (r *Registry) Conn(paymentID string) *websocket.Conn {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if e := r.conns[paymentID]; e != nil {
		r.log.Info(context.Background(), "websocket connection found", "payment_id", paymentID)
		return e.conn
	}

	return nil
}

func (r *Registry) SendJSON(paymentID string, data []byte) error {
	r.mu.RLock()
	e := r.conns[paymentID]
	r.mu.RUnlock()

	if e == nil {
		return nil
	}

	e.writeMu.Lock()
	defer e.writeMu.Unlock()

	return e.conn.WriteMessage(websocket.TextMessage, data)
}
