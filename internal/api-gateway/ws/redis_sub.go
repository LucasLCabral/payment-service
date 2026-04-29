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
		log.Info(ctx, "redis subscriber connected", "addr", redisURL, "instance_id", reg.InstanceID())
		runOneStreamConsumer(ctx, rdb, reg, log)
		_ = rdb.Close()
		if ctx.Err() != nil {
			return
		}
		log.Warn(ctx, "redis subscriber session ended, reconnecting")
		sleepCtx(ctx, backoff)
		backoff = minBackoff(backoff*2, maxBackoff)
	}
}

func runOneStreamConsumer(ctx context.Context, rdb *redis.Client, reg *Registry, log logger.Logger) {
	instanceID := reg.InstanceID()
	
	// Criar consumer group se não existir (ignora erro se já existe)
	rdb.XGroupCreate(ctx, notify.StreamPaymentNotifications, notify.ConsumerGroup, "0")
	
	log.Info(ctx, "redis streams consumer started", "stream", notify.StreamPaymentNotifications, 
		"group", notify.ConsumerGroup, "consumer", instanceID)
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Ler mensagens do stream com consumer group
			msgs, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    notify.ConsumerGroup,
				Consumer: instanceID,
				Streams:  []string{notify.StreamPaymentNotifications, ">"},
				Count:    1,
				Block:    5 * time.Second, // Timeout de 5s para evitar busy-wait
			}).Result()
			
			if err != nil {
				if err == redis.Nil {
					// Timeout normal, continua
					continue
				}
				log.Warn(ctx, "redis stream read failed", "err", err)
				return
			}
			
			// Processar mensagens recebidas
			for _, stream := range msgs {
				for _, msg := range stream.Messages {
					if err := processStreamMessage(ctx, rdb, reg, log, msg); err != nil {
						log.Error(ctx, "failed to process stream message", "err", err, "msg_id", msg.ID)
					}
				}
			}
		}
	}
}

func processStreamMessage(ctx context.Context, rdb *redis.Client, reg *Registry, log logger.Logger, msg redis.XMessage) error {
	paymentID, ok := msg.Values["payment_id"].(string)
	if !ok {
		return rdb.XAck(ctx, notify.StreamPaymentNotifications, notify.ConsumerGroup, msg.ID).Err()
	}
	
	// Verificar se esta instância tem a conexão WebSocket
	if !reg.HasConnection(paymentID) {
		// ACK a mensagem mas não processa (outra instância deve ter a conexão)
		log.Debug(ctx, "payment websocket not in this instance", "payment_id", paymentID, "instance_id", reg.InstanceID())
		return rdb.XAck(ctx, notify.StreamPaymentNotifications, notify.ConsumerGroup, msg.ID).Err()
	}
	
	// Reconstroir o payload JSON para enviar via WebSocket
	status, _ := msg.Values["status"].(string)
	declineReason, _ := msg.Values["decline_reason"].(string)
	
	payload := notify.StatusPayload{
		PaymentID:     paymentID,
		Status:        status,
		DeclineReason: declineReason,
	}
	
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Error(ctx, "failed to marshal payment status", "err", err, "payment_id", paymentID)
		return rdb.XAck(ctx, notify.StreamPaymentNotifications, notify.ConsumerGroup, msg.ID).Err()
	}
	
	// Enviar via WebSocket
	if err := reg.SendJSON(paymentID, payloadJSON); err != nil {
		log.Warn(ctx, "websocket push failed", "payment_id", paymentID, "err", err)
	} else {
		log.Info(ctx, "payment status delivered via websocket", "payment_id", paymentID, "status", status, "instance_id", reg.InstanceID())
	}
	
	// ACK a mensagem para confirmar processamento
	return rdb.XAck(ctx, notify.StreamPaymentNotifications, notify.ConsumerGroup, msg.ID).Err()
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
