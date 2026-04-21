package ws

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/trace"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type ConnRegistry interface {
	Register(paymentID string, conn *websocket.Conn)
	RemoveIf(paymentID string, conn *websocket.Conn)
}

type Handler struct {
	Reg ConnRegistry
	Log logger.Logger
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := trace.EnsureTraceID(r.Context())

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	paymentID := r.URL.Query().Get("payment_id")
	if _, err := uuid.Parse(paymentID); err != nil {
		http.Error(w, "payment_id must be a valid UUID", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.Log.Error(ctx, "websocket upgrade failed", "err", err)
		return
	}

	h.Reg.Register(paymentID, conn)
	h.Log.Info(ctx, "websocket registered", "payment_id", paymentID)

	defer func() {
		h.Reg.RemoveIf(paymentID, conn)
		_ = conn.Close()
		h.Log.Info(ctx, "websocket unregistered", "payment_id", paymentID)
	}()

	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.Log.Warn(ctx, "websocket read ended", "payment_id", paymentID, "err", err)
			}
			break
		}
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	}
}
