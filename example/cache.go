package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// cache имитирует компонент, который может упасть при запуске.
type cache struct {
	addr        string
	failStartup bool
}

func (c *cache) Startup(ctx context.Context) error {
	log.Printf("cache: connecting to %s", c.addr)

	if c.failStartup {
		return errors.New("connection refused")
	}

	if err := sleep(ctx, 50*time.Millisecond); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	log.Print("cache: ready")

	return nil
}

func (c *cache) Shutdown(context.Context) error {
	log.Print("cache: connection closed")

	return nil
}
