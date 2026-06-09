package confluence

import "context"

type skipWarmupKey struct{}

// WithSkipBackgroundWarmup marks a subscribe context so activate does not spawn duplicate OI/bootstrap work.
func WithSkipBackgroundWarmup(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipWarmupKey{}, true)
}

func skipBackgroundWarmup(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, ok := ctx.Value(skipWarmupKey{}).(bool)
	return ok && v
}
