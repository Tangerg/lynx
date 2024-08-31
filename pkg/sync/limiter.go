package sync

type Limiter struct {
	semaphore chan struct{}
}

func NewLimiter(max int) *Limiter {
	return &Limiter{
		semaphore: make(chan struct{}, max),
	}
}
func (l *Limiter) Acquire() {
	l.semaphore <- struct{}{}
}
func (l *Limiter) Release() {
	<-l.semaphore
}
