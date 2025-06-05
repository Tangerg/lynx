package middleware

type Middleware interface {
	Name() string
}

type CallMiddleware interface {
	Middleware
	Call()
}

type StreamMiddleware interface {
	Middleware
	Stream()
}
