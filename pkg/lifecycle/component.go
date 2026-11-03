package lifecycle

import (
	"context"
)

type phaseFn func(ctx context.Context) error

type component struct {
	name       string
	startupFn  phaseFn
	shutdownFn phaseFn
}

func newComponent(name string, startupFn phaseFn, shutdownFn phaseFn) *component {
	return &component{
		name:       name,
		startupFn:  startupFn,
		shutdownFn: shutdownFn,
	}
}

func (c *component) startup(ctx context.Context) error {
	return c.startupFn(ctx)
}

func (c *component) shutdown(ctx context.Context) error {
	return c.shutdownFn(ctx)
}
