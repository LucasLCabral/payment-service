package ledger

import (
	"context"
	"encoding/json"

	pkgledger "github.com/LucasLCabral/payment-service/pkg/ledger"
	"github.com/LucasLCabral/payment-service/pkg/telemetry"
	"github.com/google/uuid"
)

const settledExchange = "ledger.events"

func (s *Service) publishSettlement(ctx context.Context, paymentID uuid.UUID, decision evaluation, traceID uuid.UUID) error {
	var routingKey, status string
	switch decision.status {
	case pkgledger.EntryStatusSettled:
		routingKey = "ledger.settled.accepted"
		status = "SETTLED"
	default:
		routingKey = "ledger.settled.declined"
		status = "DECLINED"
	}

	payload, err := json.Marshal(pkgledger.SettlementResult{
		PaymentID:     paymentID,
		Status:        status,
		DeclineReason: decision.reason,
	})
	if err != nil {
		return err
	}

	headers := map[string]interface{}{
		"x-trace-id": traceID.String(),
	}
	telemetry.InjectAMQPHeaders(ctx, headers)

	if err := s.pub.Publish(ctx, settledExchange, routingKey, payload, headers); err != nil {
		s.log.Error(ctx, "settlement publish failed", "payment_id", paymentID, "routing_key", routingKey, "err", err)
		return err
	}

	s.log.Info(ctx, "settlement published", "payment_id", paymentID, "status", status, "routing_key", routingKey)
	return nil
}
