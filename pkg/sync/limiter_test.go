package sync

import (
	"fmt"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	limiter := NewLimiter(5)
	for i := 1; i < 20; i++ {
		limiter.Acquire()
		fmt.Println(i)
		go func(i int) {
			time.Sleep(time.Second * time.Duration(i))
			limiter.Release()
		}(i)
	}
}
