package bootstrap

// NotificationSource is the observation half of a composition-owned
// in-process notification. Producers receive only the relay's Publish method;
// consumers receive this interface.
type NotificationSource[T any] interface {
	Observe(func(T))
}
