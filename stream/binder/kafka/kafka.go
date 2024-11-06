package pulsar

import (
	"github.com/Tangerg/lynx/stream/binder"
	"github.com/Tangerg/lynx/stream/binding"
	"github.com/Tangerg/lynx/stream/binding/kafka"
)

type Config struct {
	URL string `yaml:"URL" json:"URL"`
}

func NewKafka(conf Config) binder.Binder {
	return &Kafka{
		config: conf,
	}
}

var _ binder.Binder = (*Kafka)(nil)

type Kafka struct {
	config Config
}

func (p *Kafka) BindProducer(destination string) (binding.Binding, error) {
	return kafka.NewKafka(kafka.Config{
		URL:       p.config.URL,
		Topic:     destination,
		Direction: binding.Send,
	}), nil
}

func (p *Kafka) BindConsumer(destination string) (binding.Binding, error) {
	return kafka.NewKafka(kafka.Config{
		URL:       p.config.URL,
		Topic:     destination,
		Direction: binding.Receive,
	}), nil
}
