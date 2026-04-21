package ws

import (
	"context"
	"encoding/json"

	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/notify"
	"github.com/redis/go-redis/v9"
)

func SubscribePaymentStatus(ctx context.Context, rdb *redis.Client, reg *Registry, log logger.Logger) {
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
