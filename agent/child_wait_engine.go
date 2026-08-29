package agent

type childWaitRegistration struct {
	parent    ProcessID
	waitID    WaitID
	spec      ChildWaitSpec
	delivered bool
}
