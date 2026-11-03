package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

// postgres имитирует компонент с небыстрым подключением к базе данных.
type postgres struct {
	dsn string
}

func (p *postgres) Startup(ctx context.Context) error {
	log.Printf("postgres: connecting to %s", p.dsn)

	if err := sleep(ctx, 300*time.Millisecond); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	log.Print("postgres: ready")

	return nil
}

func (p *postgres) Shutdown(ctx context.Context) error {
	log.Print("postgres: closing connection pool")

	if err := sleep(ctx, 100*time.Millisecond); err != nil {
		return fmt.Errorf("close pool: %w", err)
	}

	return nil
}
