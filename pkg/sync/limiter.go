package sync

// Limiter is a counting semaphore that bounds the number of operations
// running concurrently. Use [Limiter.Acquire] before starting work and
// [Limiter.Release] (typically via defer) when finished.
//
// Example:
//
//	lim := sync.NewLimiter(3)
//	for _, item := range items {
//	    go func(it Item) {
//	        lim.Acquire()
//	        defer lim.Release()
//	        process(it)
//	    }(item)
//	}
type Limiter struct {
	slots chan struct{}
}

// NewLimiter returns a Limiter that allows at most n concurrent
// operations. Panics if n <= 0.
func NewLimiter(n int) *Limiter {
	if n <= 0 {
		panic("sync: limiter capacity must be > 0")
	}
	return &Limiter{slots: make(chan struct{}, n)}
}

// Acquire reserves a slot, blocking until one is available.
func (l *Limiter) Acquire() {
	l.slots <- struct{}{}
}

// Release returns a slot, unblocking one waiting [Limiter.Acquire].
// Calling Release without a matching Acquire will block forever.
func (l *Limiter) Release() {
	<-l.slots
}
