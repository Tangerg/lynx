package broker

import (
	"context"
	"fmt"
	"github.com/Tangerg/lynx/core/message"
	"github.com/segmentio/kafka-go"
	"time"
)

type KafkaConfig struct {
	Address      string        `yaml:"Address"`
	Topic        string        `yaml:"Topic"`
	Partition    int           `yaml:"Partition"`
	WriteTimeout time.Duration `yaml:"WriteTimeout"`
	ReadTimeout  time.Duration `yaml:"ReadTimeout"`
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

// Kafka TODO 实现
type Kafka struct {
	conf *KafkaConfig
	conn *kafka.Conn
}

func (k *Kafka) Produce(ctx context.Context, msgs map[string]message.Message) error {
	//TODO implement me
	panic("implement me")
}

func (k *Kafka) Consume(ctx context.Context) (message.Message, error) {
	//TODO implement me
	panic("implement me")
}

func (k *Kafka) Ack(ctx context.Context, msg message.Message) error {
	//TODO implement me
	panic("implement me")
}

func (k *Kafka) Nack(ctx context.Context, msg message.Message) error {
	//TODO implement me
	panic("implement me")
}

func (k *Kafka) Close() error {
	//TODO implement me
	panic("implement me")
}
