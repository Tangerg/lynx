package capabilities

// DefaultModel returns the fallback model a run uses when it selects none.
func (c *Coordinator) DefaultModel() string { return c.defaultModel }

// DefaultProvider returns the fallback provider a run uses when it selects none.
func (c *Coordinator) DefaultProvider() string { return c.defaultProvider }
