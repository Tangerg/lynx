package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/identity"
	"github.com/Tangerg/lynx/app/cli/internal/resilience"
)

const (
	animationRate  = 100 * time.Millisecond
	controlTimeout = 5 * time.Second
)

func (a *app) start(message client.Message) {
	requestID, err := identity.New("req")
	if err != nil {
		a.fail(err)
		return
	}
	a.state.Starting()
	a.workflow.Reset()
	a.status.active("starting run")
	a.started = time.Now()
	a.syncAnimation()
	a.follow(func(ctx context.Context) (subscription, error) {
		run, err := a.backend.StartRun(ctx, client.StartRun{
			RequestID: requestID,
			SessionID: a.session.ID,
			Message:   cloneMessage(message),
			Options:   a.options,
		})
		if err != nil {
			return subscription{}, err
		}
		return subscription{runID: run.ID, after: run.StartedAfter}, nil
	})
}

type subscription struct {
	runID string
	after client.Cursor
}

func (a *app) follow(open func(context.Context) (subscription, error)) {
	a.dropStream()
	sequence := a.streamSeq
	ctx, cancel := context.WithCancel(a.ctx)
	a.streamCancel = cancel
	a.following = true
	dispatcher := a.loop.Dispatcher()

	go func() {
		defer cancel()
		policy := resilience.Standard(a.settings.UI.ReconnectAttempts)
		failures := 0
		var sub subscription
		for {
			var err error
			sub, err = open(ctx)
			if err == nil {
				break
			}
			if ctx.Err() != nil {
				return
			}
			failures++
			delay, retry := policy.Next(failures, err)
			if !retry {
				a.postStreamFailure(ctx, dispatcher, sequence, err)
				return
			}
			if err := post(ctx, dispatcher, func() {
				if a.streamSeq == sequence {
					a.status.note(fmt.Sprintf("reconnecting %d/%d", failures, policy.Attempts))
					a.syncAnimation()
				}
			}); err != nil {
				return
			}
			if err := resilience.Wait(ctx, delay); err != nil {
				return
			}
		}

		after := sub.after
		failures = 0
		for {
			before := after
			stream, followErr := a.backend.FollowRun(ctx, client.FollowRun{RunID: sub.runID, After: after})
			if followErr == nil && stream == nil {
				followErr = errors.New("runtime returned a nil event stream")
			}

			active := true
			phase := client.Running
			if followErr == nil {
				for envelope, streamErr := range stream {
					if streamErr != nil {
						followErr = streamErr
						break
					}
					var applyErr error
					if err := post(ctx, dispatcher, func() {
						if a.streamSeq != sequence {
							active = false
							return
						}
						applyErr = a.apply(envelope)
						after = a.state.Cursor()
						phase = a.state.Phase()
					}); err != nil || !active {
						return
					}
					if applyErr != nil {
						followErr = applyErr
						break
					}
				}
			}

			if err := post(ctx, dispatcher, func() {
				if a.streamSeq != sequence {
					active = false
					return
				}
				after = a.state.Cursor()
				phase = a.state.Phase()
			}); err != nil || !active {
				return
			}
			if phase != client.Running {
				a.finishFollowing(dispatcher, sequence)
				return
			}
			if followErr == nil {
				followErr = fmt.Errorf("%w: runtime stream ended without parking or finishing the run", client.ErrDisconnected)
			}
			if ctx.Err() != nil {
				return
			}
			if after > before {
				failures = 0
			}
			failures++
			delay, retry := policy.Next(failures, followErr)
			if !retry {
				a.postStreamFailure(ctx, dispatcher, sequence, followErr)
				return
			}
			if err := post(ctx, dispatcher, func() {
				if a.streamSeq == sequence {
					a.status.note(fmt.Sprintf("reconnecting %d/%d", failures, policy.Attempts))
					a.syncAnimation()
				}
			}); err != nil {
				return
			}
			if err := resilience.Wait(ctx, delay); err != nil {
				return
			}
		}
	}()
}

func (a *app) postStreamFailure(ctx context.Context, dispatcher program.Dispatcher, sequence uint64, err error) {
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return
	}
	_ = post(ctx, dispatcher, func() {
		if a.streamSeq == sequence {
			a.streamCancel = nil
			a.fail(err)
		}
	})
}

func (a *app) finishFollowing(dispatcher program.Dispatcher, sequence uint64) {
	_ = post(context.WithoutCancel(a.ctx), dispatcher, func() {
		if a.streamSeq == sequence {
			a.streamCancel = nil
			a.following = false
		}
	})
}

func post(ctx context.Context, dispatcher program.Dispatcher, fn func()) error {
	done := make(chan struct{}, 1)
	dispatcher.Post(func() {
		fn()
		done <- struct{}{}
	})
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-dispatcher.Done():
		return program.ErrStopped
	}
}

func (a *app) apply(envelope client.Envelope) error {
	result, err := a.state.ApplyEnvelope(envelope)
	if err != nil {
		return fmt.Errorf("apply runtime event %T at cursor %d: %w", envelope.Event, envelope.Cursor, err)
	}
	if !result.Applied {
		return nil
	}
	event := envelope.Event
	if err := a.transcript.Apply(event, a.registry); err != nil {
		return err
	}
	switch event := event.(type) {
	case client.RunStarted:
		a.status.active("working")
	case client.BlockStarted:
		if event.Block.Kind == client.BlockTool && event.Block.Tool != nil {
			a.status.active(event.Block.Tool.Summary)
		}
	case client.BlockCompleted:
		if event.Block.Kind == client.BlockTool {
			a.status.active("working")
		}
	case client.PlanChanged:
		a.workflow.Set(a.state.Plan())
	case client.RunInterrupted:
		a.openInteraction(a.state.Interaction())
		a.status.note("waiting for your answer")
	case client.RunFinished:
		a.following = false
		a.status.settled(a.state.Outcome(), a.state.Usage())
		if a.settings.UI.Notifications {
			a.loop.Session().Notify("lyra run completed")
		}
	}
	a.transcript.Retain(a.loop)
	a.syncAnimation()
	return nil
}

func (a *app) fail(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	a.following = false
	a.state.Failed(err)
	a.transcript.Append(presentError(a.transcript.theme, err.Error()))
	a.status.settled(a.state.Outcome(), a.state.Usage())
	a.syncAnimation()
}

func (a *app) cancel() {
	if a.review != nil {
		a.answerReview("deny")
		return
	}
	if a.question != nil {
		a.answerQuestion(true)
		return
	}
	if !a.state.Busy() && !a.following {
		a.loop.Quit()
		return
	}
	a.status.doing = "canceling"
	runID := a.state.RunID()
	if runID == "" {
		a.dropStream()
		a.following = false
		_ = a.state.Apply(client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCanceled}})
		a.status.settled(a.state.Outcome(), a.state.Usage())
		a.syncAnimation()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(a.ctx), controlTimeout)
		defer cancel()
		if err := a.backend.CancelRun(ctx, runID); err != nil {
			a.loop.Dispatcher().Post(func() { a.fail(err) })
		}
	}()
}

func (a *app) dropStream() {
	a.streamSeq++
	if a.streamCancel != nil {
		a.streamCancel()
		a.streamCancel = nil
	}
}

func (a *app) syncAnimation() {
	running := a.state.Phase() == client.Running
	switch {
	case running && a.stopClock == nil:
		a.stopClock = a.loop.Every(animationRate, func() {
			a.status.tick(time.Since(a.started))
		})
	case !running && a.stopClock != nil:
		a.stopClock()
		a.stopClock = nil
	}
	state := term.Progress{}
	if running {
		state.State = term.ProgressIndeterminate
	}
	a.loop.Session().SetProgress(state)
}
