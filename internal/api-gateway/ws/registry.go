package ws

import (
	"sync"

	"github.com/gorilla/websocket"
)

type Registry struct {
	mu    sync.RWMutex
	conns map[string]*websocket.Conn
}

func NewRegistry() *Registry {
	return &Registry{
		conns: make(map[string]*websocket.Conn),
	}
}

func (r *Registry) Register(paymentID string, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if oldConn, ok := r.conns[paymentID]; ok && oldConn != nil && oldConn != conn {
		_ = oldConn.Close()
	}
	r.conns[paymentID] = conn
}

func (r *Registry) RemoveIf(paymentID string, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if c, ok := r.conns[paymentID]; ok && c == conn {
		delete(r.conns, paymentID)
	}
}

func (r *Registry) Conn(paymentID string) *websocket.Conn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.conns[paymentID]
}
