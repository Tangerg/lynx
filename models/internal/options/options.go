package options

import "github.com/Tangerg/lynx/core/metadata"

func GetParams[T any](values metadata.Map, key string) (*T, error) {
	params, exists, err := metadata.Decode[T](values, key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return new(T), nil
	}
	return &params, nil
}
