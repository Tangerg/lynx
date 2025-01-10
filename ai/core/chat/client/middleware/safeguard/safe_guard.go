package safeguard

import (
	"errors"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

var ErrorSensitiveUserText = errors.New("the text entered by the user contains sensitive vocabulary")

type sensitiveWords []string

func (s sensitiveWords) check(text string) error {
	if s == nil || len(s) == 0 {
		return nil
	}

	var sensitiveWord string
	if lo.ContainsBy(s, func(word string) bool {
		if strings.Contains(text, word) {
			sensitiveWord = word
			return true
		}
		return false
	}) {
		return errors.Join(ErrorSensitiveUserText, fmt.Errorf("the sensitive word is: %s", sensitiveWord))
	}
	return nil
}

func New[O request.ChatRequestOptions, M result.ChatResultMetadata](words ...string) middleware.Middleware[O, M] {
	s := make(sensitiveWords, 0, len(words))
	s = append(s, words...)

	return func(ctx *middleware.Context[O, M]) error {
		err := s.check(ctx.Request.UserText)
		if err != nil {
			return err
		}
		return ctx.Next()
	}
}
