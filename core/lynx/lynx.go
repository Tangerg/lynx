package lynx

import (
	"context"
	"errors"
	"github.com/Tangerg/lynx/core/job"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

type Options struct {
	Jobs []job.Job
}

type Lynx struct {
	stopChan chan os.Signal
	jobs     []job.Job
}

func New(opt *Options) *Lynx {
	return &Lynx{
		jobs:     opt.Jobs,
		stopChan: make(chan os.Signal, 1),
	}
}

func (l *Lynx) start() error {
	slog.Info("-----------------")
	slog.Info("-------Lynx Start--------")
	slog.Info("-----------------")
	ctx := context.Background()
	errs := make([]error, 0, len(l.jobs))
	for _, j := range l.jobs {
		err := j.Start(ctx)
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (l *Lynx) wait() {
	slog.Info("-----------------")
	slog.Info("-------Lynx Wait--------")
	slog.Info("-----------------")
	signal.Notify(l.stopChan, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	<-l.stopChan
	close(l.stopChan)
}
func (l *Lynx) stop() error {
	slog.Info("-----------------")
	slog.Info("-------Lynx Stop--------")
	slog.Info("-----------------")
	errs := make([]error, 0, len(l.jobs))
	for _, j := range l.jobs {
		err := j.Stop()
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
