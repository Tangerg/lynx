package safeguard

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/ai/client/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/pkg/result"
	"github.com/Tangerg/lynx/pkg/stream"
)

var ErrSensitiveText = errors.New("the text entered by input contains sensitive vocabulary")

type safeGuard struct {
	sensitiveWords  []string
	passMessageType []messages.Type
}

func newSafeGuard(sensitiveWords []string, passMessageType []messages.Type) *safeGuard {
	return &safeGuard{
		sensitiveWords:  sensitiveWords,
		passMessageType: passMessageType,
	}
}

func (s *safeGuard) Check(request *chat.Request) error {
	if len(s.sensitiveWords) == 0 {
		return nil
	}

	var sb strings.Builder
	for _, message := range request.ChatRequest().Instructions() {
		if slices.Contains(s.passMessageType, message.Type()) {
			continue
		}
		sb.WriteString(message.Text())
	}

	if sb.Len() == 0 {
		return nil
	}

	text := sb.String()
	for _, sensitiveWord := range s.sensitiveWords {
		if strings.Contains(text, sensitiveWord) {
			return errors.Join(ErrSensitiveText, fmt.Errorf("the sensitive word is: %s", sensitiveWord))
		}
	}

	return nil
}

func (s *safeGuard) callMiddleware(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(request *chat.Request) (*chat.Response, error) {
		err := s.Check(request)
		if err != nil {
			return nil, err
		}
		return next.Call(request)
	})
}

func (s *safeGuard) streamMiddleware(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(request *chat.Request) (resp stream.Reader[result.Result[*chat.Response]], err error) {
		err = s.Check(request)
		if err != nil {
			return nil, err
		}
		return next.Stream(request)
	})
}

func New(sensitiveWords []string, passMessageType []messages.Type) (chat.CallMiddleware, chat.StreamMiddleware) {
	safeGauard := newSafeGuard(sensitiveWords, passMessageType)
	return safeGauard.callMiddleware, safeGauard.streamMiddleware
}
