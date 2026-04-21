// Loadclients simula vários clientes: grava saldo inicial no ledger e fica criando pagamentos
// (ok, recusado por saldo, acima do limite, idempotência, GET; opcionalmente WebSocket).
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/database"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// maxLedgerAmountCents deve coincidir com internal/ledger/evaluate.go (maxAmountCents).
const maxLedgerAmountCents = 100_000_00

var (
	merchantPayee = uuid.MustParse("feedfeed-feed-feed-feed-feedfeedfeed")
	httpClient    = &http.Client{Timeout: 20 * time.Second}
)

func main() {
	base := flag.String("base", envOr("API_BASE_URL", "http://127.0.0.1:8080"), "URL base do API Gateway")
	n := flag.Int("clients", 6, "quantidade de clientes simulados (contas com saldo)")
	duration := flag.Duration("duration", 0, "duração total (0 = até Ctrl+C)")
	tickMin := flag.Duration("tick-min", 400*time.Millisecond, "intervalo mínimo entre ações de um cliente")
	tickMax := flag.Duration("tick-max", 3*time.Second, "intervalo máximo entre ações")
	wsProb := flag.Float64("ws-prob", 0.12, "probabilidade de abrir WebSocket após criar pagamento (0–1)")
	ledgerHost := flag.String("ledger-db-host", envOr("LEDGER_DB_HOST", "localhost"), "Postgres do ledger")
	ledgerPort := flag.Int("ledger-db-port", atoiEnv("LEDGER_DB_PORT", 5433), "porta Postgres do ledger")
	ledgerUser := flag.String("ledger-db-user", envOr("LEDGER_DB_USER", "ledger_user"), "usuário")
	ledgerPass := flag.String("ledger-db-password", envOr("LEDGER_DB_PASSWORD", "ledger_pass"), "senha")
	ledgerName := flag.String("ledger-db-name", envOr("LEDGER_DB_NAME", "ledger_db"), "database")
	ledgerSSL := flag.String("ledger-db-sslmode", envOr("LEDGER_DB_SSLMODE", "disable"), "sslmode")
	flag.Parse()

	if *tickMin > *tickMax {
		log.Fatal("tick-min deve ser <= tick-max")
	}
	if *n < 1 {
		log.Fatal("clients deve ser >= 1")
	}
	if *wsProb < 0 || *wsProb > 1 {
		log.Fatal("ws-prob deve estar entre 0 e 1")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if *duration > 0 {
		go func() {
			select {
			case <-time.After(*duration):
				stop()
			case <-ctx.Done():
			}
		}()
	}

	db, err := database.Connect(ctx, database.Config{
		Host:     *ledgerHost,
		Port:     *ledgerPort,
		User:     *ledgerUser,
		Password: *ledgerPass,
		Database: *ledgerName,
		SSLMode:  *ledgerSSL,
	})
	if err != nil {
		log.Fatalf("ledger db: %v", err)
	}
	defer db.Close()

	baseURL := strings.TrimSuffix(strings.TrimSpace(*base), "/")
	clients := make([]actor, *n)
	allIDs := make([]uuid.UUID, *n)
	for i := range clients {
		id := uuid.New()
		allIDs[i] = id
		brl := randInt64(8_000, 1_800_000)
		usd := randInt64(8_000, 1_200_000)
		if err := upsertBalance(ctx, db, id, "BRL", brl); err != nil {
			log.Fatalf("seed BRL %s: %v", id, err)
		}
		if err := upsertBalance(ctx, db, id, "USD", usd); err != nil {
			log.Fatalf("seed USD %s: %v", id, err)
		}
		clients[i] = actor{
			id:      id,
			seedBRL: brl,
			seedUSD: usd,
			base:    baseURL,
			rng:     rand.New(rand.NewPCG(rand.Uint64(), uint64(i)+uint64(time.Now().UnixNano()))),
		}
		log.Printf("cliente %s saldo inicial BRL=%d USD=%d", id, brl, usd)
	}

	var wg sync.WaitGroup
	for i := range clients {
		wg.Add(1)
		go func(a *actor) {
			defer wg.Done()
			a.run(ctx, allIDs, merchantPayee, *tickMin, *tickMax, *wsProb)
		}(&clients[i])
	}

	<-ctx.Done()
	log.Println("encerrando loadclients…")
	wg.Wait()
}

type actor struct {
	id      uuid.UUID
	seedBRL int64
	seedUSD int64
	base    string
	rng     *rand.Rand
}

func (a *actor) run(ctx context.Context, pool []uuid.UUID, merchant uuid.UUID, tickMin, tickMax time.Duration, wsProb float64) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		jitter := tickMin
		if tickMax > tickMin {
			jitter += time.Duration(a.rng.Int64N(int64(tickMax - tickMin)))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(jitter):
		}

		payee := pickPayee(a.rng, pool, a.id, merchant)
		cur := "BRL"
		if a.rng.IntN(100) < 28 {
			cur = "USD"
		}
		seed := a.seedBRL
		if cur == "USD" {
			seed = a.seedUSD
		}

		switch k := a.rng.IntN(100); {
		case k < 38:
			hi := min64(25_000, max64(100, seed/4))
			amt := a.rng.Int64N(hi-99) + 100
			if hi <= 100 {
				amt = 100 + a.rng.Int64N(400)
			}
			pid, _, _ := a.postPayment(ctx, payee, cur, amt, uuid.New().String(), "settled_small")
			a.maybeWS(ctx, wsProb, pid)
		case k < 52:
			amt := seed + a.rng.Int64N(500_000) + 50_000
			_, _, _ = a.postPayment(ctx, payee, cur, amt, uuid.New().String(), "insufficient_funds")
		case k < 60:
			amt := maxLedgerAmountCents + a.rng.Int64N(9_000_000) + 1
			_, _, _ = a.postPayment(ctx, payee, cur, amt, uuid.New().String(), "over_limit")
		case k < 72:
			idem := uuid.New().String()
			amt := a.rng.Int64N(5000) + 500
			pid1, _, ok1 := a.postPayment(ctx, payee, cur, amt, idem, "idempotent_a")
			if ok1 && pid1 != "" {
				pid2, _, _ := a.postPayment(ctx, payee, cur, amt+a.rng.Int64N(100), idem, "idempotent_b")
				if pid2 != pid1 {
					log.Printf("[%s] idempotência: esperado mesmo payment_id, a=%s b=%s", a.id, pid1, pid2)
				}
			}
		default:
			idem := uuid.New().String()
			amt := a.rng.Int64N(8000) + 300
			pid, _, ok := a.postPayment(ctx, payee, cur, amt, idem, "then_get")
			if ok && pid != "" {
				a.getPayment(ctx, pid)
			}
			a.maybeWS(ctx, wsProb, pid)
		}
	}
}

func (a *actor) postPayment(ctx context.Context, payee uuid.UUID, currency string, amountCents int64, idempotencyKey, tag string) (paymentID string, idem string, ok bool) {
	body := map[string]any{
		"idempotency_key": idempotencyKey,
		"amount_cents":    amountCents,
		"currency":        currency,
		"payer_id":        a.id.String(),
		"payee_id":        payee.String(),
		"description":     fmt.Sprintf("loadclients:%s", tag),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		log.Printf("[%s] %s marshal: %v", a.id, tag, err)
		return "", idempotencyKey, false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+"/payments", bytes.NewReader(raw))
	if err != nil {
		log.Printf("[%s] %s request: %v", a.id, tag, err)
		return "", idempotencyKey, false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[%s] %s http: %v", a.id, tag, err)
		return "", idempotencyKey, false
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusAccepted {
		log.Printf("[%s] %s POST %s amount=%d cur=%s -> %s %s", a.id, tag, payee, amountCents, currency, resp.Status, truncate(string(b), 200))
		return "", idempotencyKey, false
	}

	var out struct {
		PaymentID string `json:"payment_id"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(b, &out); err != nil || out.PaymentID == "" {
		log.Printf("[%s] %s parse: %v body=%s", a.id, tag, err, truncate(string(b), 200))
		return "", idempotencyKey, false
	}

	log.Printf("[%s] %s -> payment_id=%s status=%s payer->%s %s %d", a.id, tag, out.PaymentID, out.Status, payee, currency, amountCents)
	return out.PaymentID, idempotencyKey, true
}

func (a *actor) getPayment(ctx context.Context, paymentID string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.base+"/payments/"+paymentID, nil)
	if err != nil {
		log.Printf("[%s] GET build: %v", a.id, err)
		return
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[%s] GET %s: %v", a.id, paymentID, err)
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Printf("[%s] GET %s -> %s %s", a.id, paymentID, resp.Status, truncate(string(b), 160))
		return
	}
	var out struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(b, &out)
	log.Printf("[%s] GET payment_id=%s status=%s", a.id, paymentID, out.Status)
}

func (a *actor) maybeWS(ctx context.Context, p float64, paymentID string) {
	if paymentID == "" || a.rng.Float64() > p {
		return
	}
	u, err := url.Parse(a.base)
	if err != nil {
		return
	}
	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}
	q := url.Values{}
	q.Set("payment_id", paymentID)
	wsURL := fmt.Sprintf("%s://%s/ws?%s", scheme, u.Host, q.Encode())

	dialCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	d := websocket.Dialer{HandshakeTimeout: 8 * time.Second}
	conn, _, err := d.DialContext(dialCtx, wsURL, nil)
	if err != nil {
		log.Printf("[%s] ws dial: %v", a.id, err)
		return
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	_, _, _ = conn.ReadMessage()
}

func upsertBalance(ctx context.Context, db *sql.DB, accountID uuid.UUID, currency string, amountCents int64) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO balances (account_id, currency, amount_cents, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (account_id, currency)
		DO UPDATE SET amount_cents = EXCLUDED.amount_cents, updated_at = NOW()
	`, accountID, currency, amountCents)
	return err
}

func pickPayee(rng *rand.Rand, pool []uuid.UUID, self uuid.UUID, merchant uuid.UUID) uuid.UUID {
	if len(pool) < 2 {
		return merchant
	}
	for range len(pool) + 3 {
		j := rng.IntN(len(pool))
		if pool[j] != self {
			return pool[j]
		}
	}
	return merchant
}

func randInt64(lo, hi int64) int64 {
	if hi <= lo {
		return lo
	}
	return lo + rand.Int64N(hi-lo+1)
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func atoiEnv(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
