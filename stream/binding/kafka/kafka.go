package kafka

import (
	"context"
	"errors"
	"github.com/Tangerg/lynx/stream/binding"
	"github.com/Tangerg/lynx/stream/message"
	"github.com/segmentio/kafka-go"
)

// messageID The identifier used to identify each message headers
const messageID = "message_id"

var _ binding.Binding = (*Kafka)(nil)

func NewKafka(conf Config) binding.Binding {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{conf.URL},
		Topic:          conf.Topic,
		Partition:      0,
		MinBytes:       10e3,
		CommitInterval: 0,
	})
	writer := &kafka.Writer{
		Addr:  kafka.TCP(conf.URL),
		Topic: conf.Topic,
	}
	return &Kafka{
		direction: conf.Direction,
		reader:    reader,
		writer:    writer,
	}
}

type Kafka struct {
	direction binding.Direction
	reader    *kafka.Reader
	writer    *kafka.Writer
}

func (k *Kafka) Send(ctx context.Context, message message.Message) error {
	if !k.direction.CanSend() {
		return binding.ErrorSendWithinReceiveBinding
	}

	return k.writer.WriteMessages(ctx, kafka.Message{
		Value: message.Payload(),
	})
}

func (k *Kafka) Receive(ctx context.Context) (message.Message, error) {
	if !k.direction.CanReceive() {
		return nil, binding.ErrorReceiveWithinSendBinding
	}

	msg, err := k.reader.ReadMessage(ctx)
	if err != nil {
		return nil, err
	}
	rv := message.NewSimpleMessage()
	rv.SetPayload(msg.Value).
		Headers().
		Set(messageID, msg)

	if rv.Error() != nil {
		return nil, rv.Error()
	}

	return rv, nil
}

func (k *Kafka) Ack(ctx context.Context, message message.Message) error {
	if !k.direction.CanReceive() {
		return binding.ErrorReceiveWithinSendBinding
	}
	msg, ok := message.Headers().Get(messageID)
	if !ok {
		return errors.New("ack: message not found")
	}
	kMsg, ok := msg.(kafka.Message)
	if !ok {
		return errors.New("ack: message is not kafka message")
	}

	return k.reader.CommitMessages(ctx, kMsg)
}

func (k *Kafka) Nack(_ context.Context, _ message.Message) error {
	if !k.direction.CanReceive() {
		return binding.ErrorReceiveWithinSendBinding
	}
	return nil
}

func (k *Kafka) Close() error {
	err1 := k.reader.Close()
	err2 := k.writer.Close()
	return errors.Join(err1, err2)
}
