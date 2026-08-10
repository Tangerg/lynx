package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

const (
	transcriptPaneKey = "transcript"
	promptPaneKey     = "prompt"
)

type shellView struct {
	rows       *headless.Container
	transcript *conversationView
	prompt     *promptView
}

func newShellView(
	header *sessionHeader,
	transcript *conversationView,
	activity *activityView,
	queue *queueView,
	status *statusView,
	prompt *promptView,
) *shellView {
	rows := headless.Rows(
		headless.Item{Key: "header", Size: layout.Measured(0, 2), Of: headless.Static{Of: header}},
		headless.Item{Key: transcriptPaneKey, Size: layout.Flex(1), Of: transcript},
		headless.Item{Key: "activity", Size: layout.Measured(0, activityMaxRows), Of: headless.Static{Of: activity}},
		headless.Item{Key: "queue", Size: layout.Measured(0, queueMaxRows), Of: headless.Static{Of: queue}},
		headless.Item{Key: "status", Size: layout.Fixed(1), Of: headless.Static{Of: status}},
		headless.Item{Key: promptPaneKey, Size: layout.Measured(4, 9), Of: prompt},
	)
	keys := headless.DefaultContainerKeys()
	keys.Bind(headless.FocusNext, input.Chord{Code: input.Character, Rune: ' '})
	rows.Keys = keys
	shell := &shellView{rows: rows, transcript: transcript, prompt: prompt}
	shell.focus(promptPaneKey)
	return shell
}

func (s *shellView) Draw(frame headless.Frame) { s.rows.Draw(frame) }

func (s *shellView) Handle(event input.Event) bool { return s.rows.Handle(event) }

func (s *shellView) Focus(has bool) { s.rows.Focus(has) }

func (s *shellView) PromptFocused() bool { return s.rows.Focused() == s.prompt }

func (s *shellView) TranscriptFocused() bool { return s.rows.Focused() == s.transcript }

func (s *shellView) FocusPrompt() bool { return s.focus(promptPaneKey) }

func (s *shellView) SetTranscript(transcript *conversationView) {
	items := s.rows.Items()
	for index := range items {
		if items[index].Key == transcriptPaneKey {
			items[index].Of = transcript
			break
		}
	}
	s.transcript = transcript
	s.rows.Set(items...)
}

func (s *shellView) focus(key string) bool {
	for index, item := range s.rows.Items() {
		if item.Key == key {
			return s.rows.Give(index)
		}
	}
	return false
}
