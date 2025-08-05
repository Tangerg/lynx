package logger

import (
	"strings"

	"github.com/Tangerg/lynx/ai/client/chat"
	"github.com/Tangerg/lynx/pkg/result"
	"github.com/Tangerg/lynx/pkg/stream"
)

type Logger interface {
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

type logger struct {
	logger Logger
	level  string
}

func newLogger(l Logger, level string) *logger {
	level = strings.ToUpper(strings.TrimSpace(level))
	return &logger{
		logger: l,
		level:  level,
	}
}

func (l *logger) log(log string) {
	if l.logger == nil {
		return
	}
	switch l.level {
	case "INFO":
		l.logger.Info(log)
	case "WARN":
		l.logger.Warn(log)
	case "ERROR":
		l.logger.Error(log)
	case "DEBUG":
		l.logger.Debug(log)
	default:
		l.logger.Debug(log)
	}
}

func (l *logger) logRequest(request *chat.Request) {
	l.log(request.String())
}

func (l *logger) logResponse(response *chat.Response) {
	l.log(response.String())
}

func (l *logger) logError(err error) {
	l.log(err.Error())
}

func (l *logger) callMiddleware(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(request *chat.Request) (*chat.Response, error) {
		l.logRequest(request)
		resp, err := next.Call(request)
		if err != nil {
			l.logError(err)
		} else {
			l.logResponse(resp)
		}
		return resp, err
	})
}

func (l *logger) streamMiddleware(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(request *chat.Request) (stream.Reader[result.Result[*chat.Response]], error) {
		l.logRequest(request)
		resp, err := next.Stream(request)
		if err != nil {
			l.logError(err)
		} else {
			//TODO aggregate stream response and log
		}
		return resp, err
	})
}

func New(l Logger, level string) (chat.CallMiddleware, chat.StreamMiddleware) {
	log := newLogger(l, level)
	return log.callMiddleware, log.streamMiddleware
}
