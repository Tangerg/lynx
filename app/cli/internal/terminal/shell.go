package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

const (
	transcriptPaneKey  = "transcript"
	promptPaneKey      = "prompt"
	compactShellHeight = 8
	compactShellWidth  = 12
)

type shellView struct {
	rows       *headless.Container
	transcript *transcriptView
	prompt     *promptView
	header     *sessionHeader
	activity   *activityView
	queue      *queueView
	status     *statusView
	compact    bool
}

func newShellView(
	header *sessionHeader,
	transcript *transcriptView,
	activity *activityView,
	queue *queueView,
	status *statusView,
	prompt *promptView,
) *shellView {
	shell := &shellView{
		transcript: transcript, prompt: prompt, header: header,
		activity: activity, queue: queue, status: status,
	}
	rows := headless.NewContainer(layout.Down, shell.items(false)...)
	keys := headless.DefaultContainerKeys()
	keys.Bind(headless.FocusNext, input.Chord{Code: input.Character, Rune: ' '})
	rows.Keys = keys
	shell.rows = rows
	shell.focus(promptPaneKey)
	return shell
}

func (s *shellView) Draw(frame headless.Frame) {
	width, height := frame.Size()
	s.setCompact(height < compactShellHeight || width < compactShellWidth)
	s.rows.Draw(frame)
}

func (s *shellView) Handle(event input.Event) bool { return s.rows.Handle(event) }

func (s *shellView) Focus(has bool) { s.rows.Focus(has) }

func (s *shellView) PromptFocused() bool { return s.rows.Focused() == s.prompt }

func (s *shellView) TranscriptFocused() bool { return s.rows.Focused() == s.transcript }

func (s *shellView) FocusPrompt() bool { return s.focus(promptPaneKey) }

func (s *shellView) SetTranscript(transcript *transcriptView) {
	s.transcript = transcript
	s.rows.Set(s.items(s.compact)...)
}

func (s *shellView) setCompact(compact bool) {
	if s.compact == compact {
		return
	}
	s.compact = compact
	s.prompt.SetCompact(compact)
	s.rows.Set(s.items(compact)...)
}

func (s *shellView) items(compact bool) []headless.Item {
	headerSize := layout.Measured(0, 2)
	activitySize := layout.Measured(0, activityMaxRows)
	queueSize := layout.Measured(0, queueMaxRows)
	promptSize := layout.Measured(4, 9)
	if compact {
		headerSize, activitySize, queueSize = layout.Fixed(0), layout.Fixed(0), layout.Fixed(0)
		promptSize = layout.Fixed(1)
	}
	return []headless.Item{
		{Key: "header", Size: headerSize, Of: headless.Static{Of: s.header}},
		{Key: transcriptPaneKey, Size: layout.Flex(1), Of: s.transcript},
		{Key: "activity", Size: activitySize, Of: headless.Static{Of: s.activity}},
		{Key: "queue", Size: queueSize, Of: headless.Static{Of: s.queue}},
		{Key: "status", Size: layout.Fixed(1), Of: headless.Static{Of: s.status}},
		{Key: promptPaneKey, Size: promptSize, Of: s.prompt},
	}
}

func (s *shellView) focus(key string) bool {
	for index, item := range s.rows.Items() {
		if item.Key == key {
			return s.rows.Give(index)
		}
	}
	return false
}
