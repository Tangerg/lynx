package sync

import (
	"fmt"
	"testing"
	"time"
)

func TestGo(t *testing.T) {
	Go(func() {
		time.Sleep(2 * time.Second)
		panic("err")
	}, func(err error) {
		fmt.Print(err)
	})
}
