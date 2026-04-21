package ledger

import amqp "github.com/rabbitmq/amqp091-go"

const (
	PaymentsExchange = "payments.events"
	LedgerExchange   = "ledger.events"
	Queue            = "ledger.payment.created"
	DLQQueue         = "ledger.payment.created.dlq"
	RoutingKey       = "payment.created"
)

func DeclareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(PaymentsExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}

	if err := ch.ExchangeDeclare(LedgerExchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}

	if _, err := ch.QueueDeclare(DLQQueue, true, false, false, false, nil); err != nil {
		return err
	}

	args := amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": DLQQueue,
	}
	if _, err := ch.QueueDeclare(Queue, true, false, false, false, args); err != nil {
		return err
	}

	return ch.QueueBind(Queue, RoutingKey, PaymentsExchange, false, nil)
}
