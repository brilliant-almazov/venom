package rabbitmq

import (
	"context"
	"errors"
	"fmt"

	"github.com/streadway/amqp"
)

// exchangeDeclarer is the part of *amqp.Channel that declares an exchange.
type exchangeDeclarer interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	ExchangeDeclarePassive(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
}

// declareExchange declares the exchange, or with exchange_passive only checks that it exists: a missing exchange is
// an error naming it (the broker answers 404 NOT_FOUND and closes the channel).
func (e Executor) declareExchange(ch exchangeDeclarer) error {
	if !e.ExchangePassive {
		return ch.ExchangeDeclare(e.Exchange, e.ExchangeType, e.Durable, false, false, false, nil)
	}
	err := ch.ExchangeDeclarePassive(e.Exchange, e.ExchangeType, e.Durable, false, false, false, nil)
	var amqpErr *amqp.Error
	if errors.As(err, &amqpErr) && amqpErr.Code == amqp.NotFound {
		return fmt.Errorf("exchange %q does not exist: %w", e.Exchange, err)
	}
	return err
}

// checkExchange is a step with exchange_passive and no client_type: it only checks that the exchange exists.
func (e Executor) checkExchange(ctx context.Context) error {
	if e.Exchange == "" {
		return errors.New("exchange_passive takes an exchange")
	}
	conn, ch, err := e.openChannel(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer ch.Close()
	return e.declareExchange(ch)
}
