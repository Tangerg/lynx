package protocol

import (
	"errors"
	"strings"
)

func validateProvider(provider string) error {
	if provider == "" || strings.TrimSpace(provider) != provider || strings.Contains(provider, "/") {
		return errors.New("provider is required, must not contain '/', and must not have surrounding whitespace")
	}
	return nil
}

func protocolKey(provider, name string) string {
	return provider + "/" + name
}

func protocolGeneratedToolPrefixFor(provider string) string {
	return protocolKey(provider, "generated/")
}
