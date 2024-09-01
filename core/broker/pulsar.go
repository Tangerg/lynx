package broker

import (
	"context"
	"errors"
	"fmt"
	"github.com/Tangerg/lynx/core/message"
	"github.com/apache/pulsar-client-go/pulsar"
)

type PulsarConfig struct {
	URL   string `yaml:"URL"`
	Topic string `yaml:"Topic"`
}

type Pulsar struct {
	client   pulsar.Client
	producer pulsar.Producer
	consumer pulsar.Consumer
}

func NewPulsar(conf *PulsarConfig) Broker {
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

func (p *Pulsar) Produce(ctx context.Context, msgs ...*message.Msg) error {
	if len(msgs) == 1 {
		_, err := p.producer.Send(ctx, &pulsar.ProducerMessage{
			Payload: msgs[0].Payload(),
		})
		return err
	}
	errs := make([]error, 0, len(msgs))
	for _, m := range msgs {
		_, err := p.producer.Send(ctx, &pulsar.ProducerMessage{
			Payload: m.Payload(),
		})
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (p *Pulsar) Consume(ctx context.Context) (*message.Msg, message.ID, error) {
	m, err := p.consumer.Receive(ctx)
	if err != nil {
		return nil, nil, err
	}
	return message.New(m), m.ID(), nil
}

func (p *Pulsar) Ack(ctx context.Context, id message.ID) error {
	mid, ok := id.(pulsar.MessageID)
	if !ok {
		return errors.New("ack message is not pulsar.Message")
	}
	return p.consumer.AckID(mid)
}

func (p *Pulsar) Close() error {
	p.consumer.Close()
	p.producer.Close()
	p.client.Close()
	return nil
}
