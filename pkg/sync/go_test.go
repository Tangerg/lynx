package sync

import (
	"fmt"
	"testing"
	"time"
)

func TestGo(t *testing.T) {
	Go(func() {
		time.Sleep(2 * time.Second)
		panic("custom panic error")
	}, func(err error) {
		fmt.Print(err)
	})

	time.Sleep(5 * time.Second)
}
