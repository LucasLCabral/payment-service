// Cliente de teste: POST /payments e em seguida abre WebSocket em /ws até Ctrl+C.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

func main() {
	base := flag.String("base", envOr("API_BASE_URL", "http://127.0.0.1:8080"), "URL base do API Gateway (ex.: http://localhost:8080)")
	idem := flag.String("idem", "", "idempotency_key UUID (default: gera um novo)")
	amount := flag.Int64("amount", 5000, "amount_cents")
	payer := flag.String("payer", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "payer_id UUID")
	payee := flag.String("payee", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "payee_id UUID")
	currency := flag.String("currency", "BRL", "currency")
	flag.Parse()

	idempotency := strings.TrimSpace(*idem)
	if idempotency == "" {
		idempotency = uuid.New().String()
	}

	baseURL := strings.TrimSuffix(strings.TrimSpace(*base), "/")
	createURL := baseURL + "/payments"

	body := map[string]any{
		"idempotency_key": idempotency,
		"amount_cents":    *amount,
		"currency":        *currency,
		"payer_id":        *payer,
		"payee_id":        *payee,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("POST %s", createURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewReader(raw))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		log.Fatalf("create payment: status %s body %s", resp.Status, string(respBody))
	}

	var created struct {
		PaymentID string `json:"payment_id"`
		Status      string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil {
		log.Fatalf("parse response: %v body %s", err, string(respBody))
	}
	if created.PaymentID == "" {
		log.Fatalf("missing payment_id in response: %s", string(respBody))
	}
	log.Printf("payment_id=%s status=%s", created.PaymentID, created.Status)

	u, err := url.Parse(baseURL)
	if err != nil {
		log.Fatal(err)
	}
	wsScheme := "ws"
	if u.Scheme == "https" {
		wsScheme = "wss"
	}
	host := u.Host
	q := url.Values{}
	q.Set("payment_id", created.PaymentID)
	wsURL := fmt.Sprintf("%s://%s/ws?%s", wsScheme, host, q.Encode())
	log.Printf("WebSocket %s", wsURL)

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	log.Println("WebSocket conectado; aguardando mensagens (Ctrl+C para sair)…")

	errCh := make(chan error, 1)
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			log.Printf("WS ← %s", string(msg))
		}
	}()

	select {
	case <-ctx.Done():
		log.Println("encerrando")
	case err := <-errCh:
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
			log.Println("WS:", err)
		}
	}
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
