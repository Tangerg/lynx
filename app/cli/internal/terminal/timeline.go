package terminal

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type timelineEntry struct {
	Run      agent.Run
	Position int
	Total    int
}

type timelinePane struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	picker *picker[timelineEntry]
	fork   func(timelineEntry)
}

func newTimelinePane(theme kit.Theme, glyphs kit.Glyphs, jump func(timelineEntry), fork func(timelineEntry)) *timelinePane {
	pane := &timelinePane{theme: theme, glyphs: glyphs, fork: fork}
	pane.picker = newPicker(theme, glyphs, "search runs",
		func(entry timelineEntry) string {
			return fmt.Sprintf("Run %d of %d · %s", entry.Position, entry.Total, shortIdentity(entry.Run.ID))
		},
		func(entry timelineEntry) string {
			detail := string(entry.Run.Status)
			if entry.Run.Model != "" {
				detail = entry.Run.Model + " · " + detail
			}
			return detail
		},
		jump,
	)
	return pane
}

func (p *timelinePane) SetRuns(runs []agent.Run) {
	entries := make([]timelineEntry, 0, len(runs))
	for index, run := range runs {
		entries = append(entries, timelineEntry{Run: run.Clone(), Position: index + 1, Total: len(runs)})
	}
	slices.Reverse(entries)
	p.picker.Reset()
	p.picker.SetItems(entries)
}

func (p *timelinePane) Draw(frame headless.Frame) {
	rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Flex(1)}, {Size: layout.Fixed(1)},
	}))
	p.picker.Draw(rows[0])
	kit.Label{Text: "enter jump to retained output · alt+f fork from run · esc close", Style: p.theme.Subtle, Ellipsis: p.glyphs.Ellipsis}.Draw(rows[1].View)
}

func (p *timelinePane) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() && key.Mods == input.Alt && key.Rune == 'f' {
		if entry, selected := p.picker.Current(); selected && p.fork != nil {
			p.fork(entry)
		}
		return true
	}
	return p.picker.Handle(event)
}

func (p *timelinePane) Focus(has bool) { p.picker.Focus(has) }

func (a *app) buildTimeline(theme kit.Theme, glyphs kit.Glyphs) {
	a.timeline = newTimelinePane(theme, glyphs,
		func(entry timelineEntry) {
			a.timelineDialog.Dismiss()
			if !a.transcript.JumpToRun(entry.Run.ID) {
				a.message("that run no longer has retained transcript output")
				return
			}
			a.shell.focus(transcriptPaneKey)
		},
		func(entry timelineEntry) {
			a.timelineDialog.Dismiss()
			a.forkSessionFromRun(entry.Run.ID)
		},
	)
	a.timelineDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Current session timeline", Body: a.timeline,
		Where: layout.Placement{Width: 88, Height: 20},
	})
	a.timeline.picker.cancel = a.timelineDialog.Dismiss
}

func (a *app) ShowTimeline() {
	if a.conversation.Busy() || a.following {
		a.message("finish or cancel the current run before opening the timeline")
		return
	}
	sessionID := a.session.ID
	a.message("loading timeline")
	runOperation(a, pickerCatalogOperation, true,
		func(ctx context.Context) (agent.SessionSnapshot, error) { return a.runtime.GetSession(ctx, sessionID) },
		func(snapshot agent.SessionSnapshot, err error) {
			if err != nil {
				a.message("could not load timeline: " + err.Error())
				return
			}
			if len(snapshot.Runs) == 0 {
				a.message("the current session has no runs")
				return
			}
			a.timeline.SetRuns(snapshot.Runs)
			a.timelineDialog.Show()
		},
	)
}

func shortIdentity(identity string) string {
	identity = strings.TrimSpace(identity)
	if len(identity) <= 12 {
		return identity
	}
	return identity[:12]
}
