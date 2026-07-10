package runtime

import "errors"

// Close cancels live turns and maintenance tasks, then releases the engine and
// injected process resources. It is idempotent.
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		if r.turns != nil {
			r.turns.Close()
		}
		r.tasks.Close()
		var errs []error
		if r.closer != nil {
			errs = append(errs, r.closer.Close())
		}
		for _, resource := range r.resources {
			if resource != nil {
				errs = append(errs, resource.Close())
			}
		}
		r.resources = nil
		r.closeErr = errors.Join(errs...)
	})
	return r.closeErr
}
