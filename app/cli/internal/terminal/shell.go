package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

type shellView struct {
	rows *headless.Container
}

func newShellView(
	header *sessionHeader,
	transcript *conversationView,
	activity *activityView,
	status *statusView,
	prompt *promptView,
) *shellView {
	rows := headless.Rows(
		headless.Item{Key: "header", Size: layout.Measured(0, 2), Of: headless.Static{Of: header}},
		headless.Item{Key: "transcript", Size: layout.Flex(1), Of: transcript},
		headless.Item{Key: "activity", Size: layout.Measured(0, activityMaxRows), Of: headless.Static{Of: activity}},
		headless.Item{Key: "status", Size: layout.Fixed(1), Of: headless.Static{Of: status}},
		headless.Item{Key: "prompt", Size: layout.Measured(4, 9), Of: prompt},
	)
	return &shellView{rows: rows}
}

func (s *shellView) Draw(frame headless.Frame) { s.rows.Draw(frame) }

func (s *shellView) Handle(event input.Event) bool { return s.rows.Handle(event) }

func (s *shellView) Focus(has bool) { s.rows.Focus(has) }

func (s *shellView) SetTranscript(transcript *conversationView) {
	items := s.rows.Items()
	for index := range items {
		if items[index].Key == "transcript" {
			items[index].Of = transcript
			break
		}
	}
	s.rows.Set(items...)
}
