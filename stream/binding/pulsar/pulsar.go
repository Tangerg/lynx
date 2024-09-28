package pulsar

import (
	"context"
	"fmt"

	"github.com/apache/pulsar-client-go/pulsar"

	"github.com/Tangerg/lynx/stream/binding"
	"github.com/Tangerg/lynx/stream/message"
)

// messageID The identifier used to identify each message headers
const messageID = "message_id"

type Config struct {
	URL       string            `json:"URL"`
	Topic     string            `json:"Topic"`
	Direction binding.Direction `json:"Direction"`
}

func NewPulsar(conf Config) binding.Binding {
	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL: conf.URL,
	})
	if err != nil {
		panic(fmt.Sprintf("create pulsar client failed: %v", err))
	}
	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic: conf.Topic,
	})
	if err != nil {
		panic(fmt.Sprintf("create pulsar producer failed: %v", err))
	}
	consumer, err := client.Subscribe(pulsar.ConsumerOptions{
		Topic: conf.Topic,
	})
	if err != nil {
		panic(fmt.Sprintf("create pulsar consumer failed: %v", err))
	}
	return &Pulsar{
		client:   client,
		producer: producer,
		consumer: consumer,
	}
}

type Pulsar struct {
	direction binding.Direction
	client    pulsar.Client
	producer  pulsar.Producer
	consumer  pulsar.Consumer
}

func (p *Pulsar) Send(ctx context.Context, message message.Message) error {
	if p.direction == binding.Receive {
		return binding.ErrorSendWithinReceiveBinding
	}

	_, err := p.producer.Send(
		ctx,
		&pulsar.ProducerMessage{
			Payload: message.Payload(),
		})
	return err
}

func (p *Pulsar) Receive(ctx context.Context) (message.Message, error) {
	if p.direction == binding.Send {
		return nil, binding.ErrorReceiveWithinSendBinding
	}

	msg, err := p.consumer.Receive(ctx)
	if err != nil {
		return nil, err
	}
	rv := message.NewSimpleMessage()
	rv.SetPayload(msg.Payload()).
		Headers().
		Set(messageID, msg.ID())

	if rv.Error() != nil {
		return nil, rv.Error()
	}

	return rv, nil
}

// getMid TODO  handle error
func (p *Pulsar) getMid(msg message.Message) (pulsar.MessageID, error) {
	headers := msg.Headers()
	if headers == nil {
		return nil, nil
	}
	mid, ok := headers.Get(messageID)
	if !ok {
		return nil, nil
	}
	if mid == nil {
		return nil, nil
	}
	pmid, ok := mid.(pulsar.MessageID)
	if !ok {
		return nil, nil
	}
	return pmid, nil
}

func (p *Pulsar) Ack(ctx context.Context, msg message.Message) error {
	mid, err := p.getMid(msg)
	if err != nil {
		return err
	}
	return p.consumer.AckID(mid)
}

func (p *Pulsar) Nack(ctx context.Context, msg message.Message) error {
	mid, err := p.getMid(msg)
	if err != nil {
		return err
	}
	p.consumer.NackID(mid)
	return nil
}
