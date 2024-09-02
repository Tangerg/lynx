package broker

import (
	"context"
	"errors"
	"fmt"
	"github.com/Tangerg/lynx/core/message"
	"github.com/apache/pulsar-client-go/pulsar"
	"sync"
)

type PulsarConfig struct {
	URL   string `yaml:"URL"`
	Topic string `yaml:"Topic"`
}

func NewPulsar(conf *PulsarConfig) Broker {
	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL: conf.URL,
	})
	if err != nil {
		panic(fmt.Sprintf("create pulsar client failed: %v", err))
	}
	consumer, err := client.Subscribe(pulsar.ConsumerOptions{
		Topic: conf.Topic,
	})
	if err != nil {
		panic(fmt.Sprintf("create pulsar consumer failed: %v", err))
	}
	return &Pulsar{
		client:    client,
		producers: make(map[string]pulsar.Producer),
		consumer:  consumer,
	}
}

type Pulsar struct {
	mu        sync.RWMutex
	client    pulsar.Client
	producers map[string]pulsar.Producer
	consumer  pulsar.Consumer
}

func (p *Pulsar) getProducer(topic string) (pulsar.Producer, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	producer, ok := p.producers[topic]
	if ok {
		return producer, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	producer, err := p.client.CreateProducer(pulsar.ProducerOptions{
		Topic: topic,
	})
	if err != nil {
		return nil, err
	}
	p.producers[topic] = producer
	return producer, nil
}

func (p *Pulsar) produce(ctx context.Context, topic string, msg message.Message) error {
	producer, err := p.getProducer(topic)
	if err != nil {
		return err
	}
	_, err = producer.Send(
		ctx,
		&pulsar.ProducerMessage{
			Payload: msg.Payload(),
		},
	)
	return err
}

func (p *Pulsar) Produce(ctx context.Context, msgs map[string]message.Message) error {
	errs := make([]error, 0, len(msgs))
	for topic, msg := range msgs {
		err := p.produce(ctx, topic, msg)
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (p *Pulsar) Consume(ctx context.Context) (message.Message, error) {
	msg, err := p.consumer.Receive(ctx)
	if err != nil {
		return nil, err
	}

	rv := message.
		NewSimpleMessage().
		SetPayload(msg.Payload()).
		SetHeaders(message.
			NewHeaders().
			Set(messageID, msg.ID()),
		)
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

func (p *Pulsar) Close() error {
	p.consumer.Close()
	for _, producer := range p.producers {
		producer.Close()
	}
	p.client.Close()
	return nil
}
