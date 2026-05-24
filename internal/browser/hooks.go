package browser

import "context"

// HookName identifies a point in the crawl lifecycle where user code can run.
type HookName string

const (
	HookBeforeNavigate HookName = "before_navigate"
	HookAfterNavigate  HookName = "after_navigate"
	HookBeforeExtract  HookName = "before_extract"
	HookAfterExtract   HookName = "after_extract"
	HookBeforeScroll   HookName = "before_scroll"
	HookAfterScroll    HookName = "after_scroll"
	HookOnError        HookName = "on_error"
	HookBeforeClose    HookName = "before_close"
)

// HookFunc is a function called at a lifecycle point. It receives the session
// and current URL. Returning an error from a pre-hook aborts the operation.
type HookFunc func(ctx context.Context, sess *cdpSession, url string) error

// Hooks holds registered lifecycle callbacks.
type Hooks struct {
	hooks map[HookName][]HookFunc
}

// NewHooks creates an empty hook registry.
func NewHooks() *Hooks {
	return &Hooks{hooks: make(map[HookName][]HookFunc)}
}

// Register adds a hook function for the given lifecycle point.
func (h *Hooks) Register(name HookName, fn HookFunc) {
	h.hooks[name] = append(h.hooks[name], fn)
}

// Run executes all hooks for the given name in registration order.
// Returns the first error encountered; subsequent hooks are skipped.
func (h *Hooks) Run(ctx context.Context, name HookName, sess *cdpSession, url string) error {
	for _, fn := range h.hooks[name] {
		if err := fn(ctx, sess, url); err != nil {
			return err
		}
	}
	return nil
}
