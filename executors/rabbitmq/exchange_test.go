package rabbitmq

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/streadway/amqp"

	"github.com/ovh/venom"
	"github.com/ovh/venom/internal/golden"
)

// fakeChannel records the declares and answers the passive one by the exchanges it has.
type fakeChannel struct {
	exists map[string]bool
	calls  []string
}

func (f *fakeChannel) ExchangeDeclare(name, kind string, durable, _, _, _ bool, _ amqp.Table) error {
	f.calls = append(f.calls, fmt.Sprintf("declare %s %s durable=%t", name, kind, durable))
	return nil
}

func (f *fakeChannel) ExchangeDeclarePassive(name, kind string, durable, _, _, _ bool, _ amqp.Table) error {
	f.calls = append(f.calls, fmt.Sprintf("passive %s %s durable=%t", name, kind, durable))
	if f.exists[name] {
		return nil
	}
	return &amqp.Error{Code: amqp.NotFound, Reason: "NOT_FOUND - no exchange '" + name + "' in vhost '/'"}
}

func TestDeclareExchangePassive(t *testing.T) {
	ch := &fakeChannel{exists: map[string]bool{"crawler.tasks": true}}
	var lines []string
	for _, e := range []Executor{
		{Exchange: "crawler.tasks", ExchangeType: "topic", Durable: true, ExchangePassive: true},
		{Exchange: "crawler.missing", ExchangeType: "topic", Durable: true, ExchangePassive: true},
		{Exchange: "crawler.own", ExchangeType: "fanout"},
	} {
		err := e.declareExchange(ch)
		lines = append(lines, fmt.Sprintf("%s: %v", e.Exchange, err))
	}
	golden.Text(t, "exchange_passive", strings.Join(append(ch.calls, lines...), "\n")+"\n")
}

func TestExchangePassiveNeedsExchange(t *testing.T) {
	_, err := Executor{}.Run(context.Background(), venom.TestStep{"addrs": "amqp://localhost:1", "exchange_passive": true})
	if err == nil || !strings.Contains(err.Error(), "takes an exchange") {
		t.Fatalf("err %v", err)
	}
	_, err = Executor{}.Run(context.Background(), venom.TestStep{"addrs": "amqp://localhost:1"})
	if err == nil || !strings.Contains(err.Error(), "must be publisher") {
		t.Fatalf("err %v", err)
	}
}
