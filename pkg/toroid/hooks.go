package toroid

import "context"

type HookFn func(ctx context.Context, e Event) error

type HookRegistry struct {
	hooks []struct {
		kind EventKind
		fn   HookFn
	}
}

func (r *HookRegistry) On(kind EventKind, fn HookFn) {
	r.hooks = append(r.hooks, struct {
		kind EventKind
		fn   HookFn
	}{kind, fn})
}

// Fire runs all registered hooks for the event kind in order.
// A non-nil error from any hook aborts the chain and is returned.
func (r *HookRegistry) Fire(ctx context.Context, e Event) error {
	for _, h := range r.hooks {
		if h.kind == e.Kind {
			if err := h.fn(ctx, e); err != nil {
				return err
			}
		}
	}
	return nil
}
