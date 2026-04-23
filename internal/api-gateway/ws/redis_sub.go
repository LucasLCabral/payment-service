package ws

import (
	"context"
	"encoding/json"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/notify"
	"github.com/redis/go-redis/v9"
)

func SubscribePaymentStatus(ctx context.Context, redisURL string, reg *Registry, log logger.Logger) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		rdb, err := notify.ConnectRedis(ctx, redisURL)
		if err != nil {
			log.Warn(ctx, "redis subscriber connect failed", "err", err)
			sleepCtx(ctx, backoff)
			backoff = minBackoff(backoff*2, maxBackoff)
			continue
		}
		backoff = time.Second
		log.Info(ctx, "redis subscriber connected", "addr", redisURL)
		runOneSubscription(ctx, rdb, reg, log)
		_ = rdb.Close()
		if ctx.Err() != nil {
			return
		}
		log.Warn(ctx, "redis subscriber session ended, reconnecting")
		sleepCtx(ctx, backoff)
		backoff = minBackoff(backoff*2, maxBackoff)
	}
}

func runOneSubscription(ctx context.Context, rdb *redis.Client, reg *Registry, log logger.Logger) {
	sub := rdb.Subscribe(ctx, notify.ChannelPaymentStatus)
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if msg == nil {
				continue
			}

			var p notify.StatusPayload
			if err := json.Unmarshal([]byte(msg.Payload), &p); err != nil {
				log.Warn(ctx, "redis payment status invalid json", "err", err, "payload", msg.Payload)
				continue
			}

			if err := reg.SendJSON(p.PaymentID, []byte(msg.Payload)); err != nil {
				log.Warn(ctx, "websocket push failed", "payment_id", p.PaymentID, "err", err)
			}
			log.Info(ctx, "redis payment status received", "payment_id", p.PaymentID, "status", p.Status)
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func minBackoff(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
