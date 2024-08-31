package lynx

import (
	"context"
	"github.com/Tangerg/lynx/core/scheduler"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

type Lynx struct {
	stopChan  chan os.Signal
	scheduler *scheduler.Scheduler
}

func New(s *scheduler.Scheduler) *Lynx {
	return &Lynx{
		scheduler: s,
		stopChan:  make(chan os.Signal, 1),
	}
}

func (l *Lynx) start() {
	slog.Info("-----------------")
	slog.Info("-------Lynx Start--------")
	slog.Info("-----------------")
	l.scheduler.Start(context.Background())
}
func (l *Lynx) wait() {
	signal.Notify(l.stopChan, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	<-l.stopChan
	close(l.stopChan)
}
func (l *Lynx) stop() {
	slog.Info("-----------------")
	slog.Info("-------Lynx Stop--------")
	slog.Info("-----------------")
	l.scheduler.Stop()
}
