package runtime

// DefaultModel is the model a turn runs against when it doesn't pick one
// (the configured Config.Model seed). The session layer uses it to fill
// Session.model for sessions that never explicitly selected a model, so the
// wire always carries a real model name. May be empty if unconfigured.
func (r *Runtime) DefaultModel() string { return r.defaultModel }

// DefaultProvider is the provider a turn runs against when a run names none
// (paired with DefaultModel). usage.summary uses it to attribute default-model
// runs (whose RunRef carries no provider) to the real provider. May be empty.
func (r *Runtime) DefaultProvider() string { return r.defaultProvider }
