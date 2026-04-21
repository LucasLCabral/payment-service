package outbox

import "github.com/LucasLCabral/payment-service/pkg/messaging"

func DeclareExchange(pub *messaging.Publisher) error {
	return pub.Channel().ExchangeDeclare(
		exchange, // payments.events
		"topic",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	)
}
