package pulsar

import "github.com/Tangerg/lynx/stream/binding"

type Config struct {
	URL       string            `json:"URL" yaml:"URL"`
	Topic     string            `json:"Topic" yaml:"Topic"`
	Direction binding.Direction `json:"Direction" yaml:"Direction"`
}
