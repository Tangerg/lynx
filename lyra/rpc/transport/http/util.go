package http

import "time"

// nowFn is the time source used by the replay buffer — swappable in
// tests if we ever want to verify the 30s eviction explicitly.
var nowFn = time.Now
