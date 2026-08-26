package tokenizer_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/tokenizer"
)

type wordEstimator struct{}

func (wordEstimator) EstimateText(_ context.Context, text string) (int, error) {
	return len(strings.Fields(text)), nil
}

func Example() {
	var estimator tokenizer.TextEstimator = wordEstimator{}
	count, err := estimator.EstimateText(context.Background(), "small stable contract")
	if err != nil {
		panic(err)
	}
	fmt.Println(count)
	// Output: 3
}
