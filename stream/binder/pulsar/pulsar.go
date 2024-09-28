package pulsar

import (
	"github.com/Tangerg/lynx/stream/binder"
	"github.com/Tangerg/lynx/stream/binding"
	"github.com/Tangerg/lynx/stream/binding/pulsar"
)

type Config struct {
	URL string `yaml:"URL"`
}

func NewPulsar(conf Config) binder.Binder {
	return &Pulsar{
		config: conf,
	}
}

type Pulsar struct {
	config Config
}

func (p *Pulsar) BindProducer(destination string) (binding.Binding, error) {
	return pulsar.NewPulsar(pulsar.Config{
		URL:       p.config.URL,
		Topic:     destination,
		Direction: binding.Send,
	}), nil
}

func (p *Pulsar) BindConsumer(destination string) (binding.Binding, error) {
	return pulsar.NewPulsar(pulsar.Config{
		URL:       p.config.URL,
		Topic:     destination,
		Direction: binding.Receive,
	}), nil
}
