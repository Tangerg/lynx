package runflow

import (
	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
)

type SteerWrite struct {
	Run               rundomain.Record
	ExpectedSegmentID string
	Item              transcript.Record
	Message           conversationdomain.Record
	Event             rundomain.EventRecord
}
