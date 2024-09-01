package broker

import (
	"context"
	"errors"
	"fmt"
	"github.com/Tangerg/lynx/core/message"
	"github.com/segmentio/kafka-go"
	"time"
)

//TODO 实现

type KafkaConfig struct {
	Address      string        `yaml:"Address"`
	Topic        string        `yaml:"Topic"`
	Partition    int           `yaml:"Partition"`
	WriteTimeout time.Duration `yaml:"WriteTimeout"`
	ReadTimeout  time.Duration `yaml:"ReadTimeout"`
}

type Kafka struct {
	conf *KafkaConfig
	conn *kafka.Conn
}

func NewKafka(conf *KafkaConfig) Broker {
	conn, err := kafka.DialLeader(
		context.Background(),
		"tcp",
		conf.Address,
		conf.Topic,
		conf.Partition,
	)
	if err != nil {
		panic(fmt.Sprintf("failed to dial Kafka broker: %s", err))
	}
	return &Kafka{
		conn: conn,
	}
}

func (k *Kafka) Produce(ctx context.Context, msgs ...*message.Msg) error {
	if len(msgs) == 1 {
		_, err := k.conn.Write(msgs[0].Payload())
		return err
	}

	errs := make([]error, 0, len(msgs))
	for _, m := range msgs {
		_, err := k.conn.Write(m.Payload())
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (k *Kafka) Consume(ctx context.Context) (*message.Msg, message.ID, error) {
	payload := make([]byte, 0)
	_, err := k.conn.Read(payload)
	return message.New(payload), nil, err
}

func (k *Kafka) Ack(ctx context.Context, id message.ID) error {
	return nil
}

func (k *Kafka) Close() error {
	return k.conn.Close()
}
