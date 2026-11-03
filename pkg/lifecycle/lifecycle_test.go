package lifecycle_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/RomanDeveloperGit/go-lifecycle/pkg/lifecycle"
)

type TestComponent struct {
	startupCalls  int
	shutdownCalls int
	startupErr    error
	shutdownErr   error
}

func (c *TestComponent) assertStartupCalled(t *testing.T, n int) {
	t.Helper()
	if c.startupCalls != n {
		t.Errorf("startupCalls = %d, want %d", c.startupCalls, n)
	}
}

func (c *TestComponent) assertShutdownCalled(t *testing.T, n int) {
	t.Helper()
	if c.shutdownCalls != n {
		t.Errorf("shutdownCalls = %d, want %d", c.shutdownCalls, n)
	}
}

func (c *TestComponent) Startup(ctx context.Context) error {
	done := make(chan struct{}, 1)

	go func() {
		time.Sleep(time.Millisecond * 250)
		close(done)
	}()

	c.startupCalls++

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return c.startupErr
	}
}

func (c *TestComponent) Shutdown(ctx context.Context) error {
	done := make(chan struct{}, 1)

	go func() {
		time.Sleep(time.Millisecond * 500)
		close(done)
	}()

	c.shutdownCalls++

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return c.shutdownErr
	}
}

func TestAddComponent(t *testing.T) {
	type input struct {
		name      string
		component lifecycle.Component
	}

	tests := []struct {
		name  string
		setup func(l *lifecycle.Lifecycle)
		input input
		want  error
	}{
		{
			"success",
			nil,
			input{"PostgreSQL", &TestComponent{}},
			nil,
		},
		{
			"empty name",
			nil,
			input{"", &TestComponent{}},
			lifecycle.ErrComponentNameIsEmpty,
		},
		{
			"nil component",
			nil,
			input{"Kafka", nil},
			lifecycle.ErrComponentIsNil,
		},
		{
			"duplicate component name",
			func(l *lifecycle.Lifecycle) {
				l.AddComponent("Redis", &TestComponent{})
			},
			input{"Redis", &TestComponent{}},
			lifecycle.ErrComponentNameAlreadyExists,
		},
		{
			"add after startup",
			func(l *lifecycle.Lifecycle) {
				l.Startup(context.Background())
			},
			input{"ClickHouse", &TestComponent{}},
			lifecycle.ErrLifecycleIsSealed,
		},
		{
			"add after shutdown",
			func(l *lifecycle.Lifecycle) {
				l.Shutdown(context.Background())
			},
			input{"RabbitMQ", &TestComponent{}},
			lifecycle.ErrLifecycleIsSealed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lifecycle.New()

			if tt.setup != nil {
				tt.setup(l)
			}

			got := l.AddComponent(tt.input.name, tt.input.component)

			if !errors.Is(got, tt.want) {
				t.Fatalf("got err=%v, want err=%v", got, tt.want)
			}
		})
	}
}

func CreateMocksAndLifecycle() (map[string]*TestComponent, *lifecycle.Lifecycle) {
	mocks := map[string]*TestComponent{
		"PostgreSQL": {},
		"Kafka":      {},
		"Redis":      {},
	}

	l := lifecycle.New()
	l.AddComponent("PostgreSQL", mocks["PostgreSQL"])
	l.AddComponent("Kafka", mocks["Kafka"])
	l.AddComponent("Redis", mocks["Redis"])

	return mocks, l
}

func TestStartup(t *testing.T) {
	mocks, l := CreateMocksAndLifecycle()

	reports, err := l.Startup(context.Background())
	if err != nil {
		t.Fatalf("Startup() error = %v, want nil", err)
	}

	// При старте компоненты должны быть в FILO порядке
	wantOrder := []string{"PostgreSQL", "Kafka", "Redis"}

	if len(reports) != len(wantOrder) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(wantOrder))
	}

	for i, want := range wantOrder {
		if got := reports[i].ComponentName; got != want {
			t.Errorf("reports[%d].ComponentName = %q, want %q", i, got, want)
		}
	}

	for _, mock := range mocks {
		mock.assertStartupCalled(t, 1)
	}
}

func TestStartup_WithErrors(t *testing.T) {
	mocks, l := CreateMocksAndLifecycle()

	pgErr := errors.New("PostgreSQL startup error")
	redisErr := errors.New("Redis startup error")

	mocks["PostgreSQL"].startupErr = pgErr
	mocks["Redis"].startupErr = redisErr

	reports, err := l.Startup(context.Background())

	if len(reports) != len(mocks) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(mocks))
	}

	commonErr := errors.Join(pgErr, redisErr)

	if err == nil {
		t.Fatalf("Startup() error = nil, want %v", commonErr)
	}

	if !errors.Is(err, pgErr) || !errors.Is(err, redisErr) {
		t.Fatalf("Startup() error = %v, want %v", err, commonErr)
	}
}

func TestStartup_WithContextCancel(t *testing.T) {
	mocks, l := CreateMocksAndLifecycle()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reports, err := l.Startup(ctx)

	if len(reports) != len(mocks) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(mocks))
	}

	if err == nil {
		t.Fatalf("Startup() error = nil, want context canceled")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Startup() error = %v, want context canceled", err)
	}
}

func TestStartup_WithContextTimeout(t *testing.T) {
	mocks, l := CreateMocksAndLifecycle()

	// успеет запуститься только PostgreSQL, Kafka прервется на половине, а Redis скипнется
	ctx, cancel := context.WithTimeout(context.Background(), 375*time.Millisecond)
	defer cancel()

	reports, err := l.Startup(ctx)

	if len(reports) != len(mocks) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(mocks))
	}

	if err == nil {
		t.Fatalf("Startup() error = nil, want context timeout")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Startup() error = %v, want context timeout", err)
	}

	if reports[0].Err != nil {
		t.Fatalf("reports[0].Error = %v, want nil", reports[0].Err)
	}
}

func TestShutdown(t *testing.T) {
	mocks := map[string]*TestComponent{
		"PostgreSQL": {},
		"Kafka":      {},
		"Redis":      {},
	}

	l := lifecycle.New()
	l.AddComponent("PostgreSQL", mocks["PostgreSQL"])
	l.AddComponent("Kafka", mocks["Kafka"])
	l.AddComponent("Redis", mocks["Redis"])

	reports, err := l.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown() error = %v, want nil", err)
	}

	// При стопе компоненты должны быть в LIFO порядке
	wantOrder := []string{"Redis", "Kafka", "PostgreSQL"}

	if len(reports) != len(wantOrder) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(wantOrder))
	}

	for i, want := range wantOrder {
		if got := reports[i].ComponentName; got != want {
			t.Errorf("reports[%d].ComponentName = %q, want %q", i, got, want)
		}
	}

	for _, mock := range mocks {
		mock.assertShutdownCalled(t, 1)
	}
}

func TestShutdown_WithErrors(t *testing.T) {
	mocks, l := CreateMocksAndLifecycle()

	pgErr := errors.New("PostgreSQL shutdown error")
	redisErr := errors.New("Redis shutdown error")

	mocks["PostgreSQL"].shutdownErr = pgErr
	mocks["Redis"].shutdownErr = redisErr

	reports, err := l.Shutdown(context.Background())

	if len(reports) != len(mocks) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(mocks))
	}

	commonErr := errors.Join(pgErr, redisErr)

	if err == nil {
		t.Fatalf("Shutdown() error = nil, want %v", commonErr)
	}

	if !errors.Is(err, pgErr) || !errors.Is(err, redisErr) {
		t.Fatalf("Shutdown() error = %v, want %v", err, commonErr)
	}
}

func TestShutdown_WithContextCancel(t *testing.T) {
	mocks, l := CreateMocksAndLifecycle()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reports, err := l.Shutdown(ctx)

	if len(reports) != len(mocks) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(mocks))
	}

	if err == nil {
		t.Fatalf("Shutdown() error = nil, want context canceled")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown() error = %v, want context canceled", err)
	}
}

func TestShutdown_WithContextTimeout(t *testing.T) {
	mocks, l := CreateMocksAndLifecycle()

	// успеет остановиться только PostgreSQL, Kafka прервется на половине, а Redis скипнется
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()

	reports, err := l.Shutdown(ctx)

	if len(reports) != len(mocks) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(mocks))
	}

	if err == nil {
		t.Fatalf("Shutdown() error = nil, want context timeout")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want context timeout", err)
	}
}
