package lifecycle_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/RomanDeveloperGit/go-lifecycle/pkg/lifecycle"
)

// fakeComponent - тестовый компонент, который фиксирует вызовы и умеет возвращать заданные ошибки.
type fakeComponent struct {
	name   string
	events *[]string // общий журнал вызовов, чтобы проверять реальный порядок, а не только порядок отчетов

	startupCalls  int
	shutdownCalls int

	startupErr  error
	shutdownErr error

	// Если true, то Startup/Shutdown блокируются до отмены контекста
	blockOnStartup  bool
	blockOnShutdown bool
}

func (c *fakeComponent) Startup(ctx context.Context) error {
	c.startupCalls++
	c.record("startup")

	if c.blockOnStartup {
		<-ctx.Done()
		return ctx.Err()
	}

	return c.startupErr
}

func (c *fakeComponent) Shutdown(ctx context.Context) error {
	c.shutdownCalls++
	c.record("shutdown")

	if c.blockOnShutdown {
		<-ctx.Done()
		return ctx.Err()
	}

	return c.shutdownErr
}

func (c *fakeComponent) record(op string) {
	if c.events != nil {
		*c.events = append(*c.events, op+":"+c.name)
	}
}

func (c *fakeComponent) assertStartupCalled(t *testing.T, n int) {
	t.Helper()

	if c.startupCalls != n {
		t.Errorf("%s: startupCalls = %d, want %d", c.name, c.startupCalls, n)
	}
}

func (c *fakeComponent) assertShutdownCalled(t *testing.T, n int) {
	t.Helper()

	if c.shutdownCalls != n {
		t.Errorf("%s: shutdownCalls = %d, want %d", c.name, c.shutdownCalls, n)
	}
}

// fixture - Lifecycle с тремя зарегистрированными компонентами в порядке PostgreSQL -> Kafka -> Redis.
type fixture struct {
	lifecycle *lifecycle.Lifecycle
	events    *[]string

	postgres *fakeComponent
	kafka    *fakeComponent
	redis    *fakeComponent
}

func (f *fixture) all() []*fakeComponent {
	return []*fakeComponent{f.postgres, f.kafka, f.redis}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	events := &[]string{}

	f := &fixture{
		lifecycle: lifecycle.New(),
		events:    events,
		postgres:  &fakeComponent{name: "PostgreSQL", events: events},
		kafka:     &fakeComponent{name: "Kafka", events: events},
		redis:     &fakeComponent{name: "Redis", events: events},
	}

	for _, c := range f.all() {
		mustAddComponent(t, f.lifecycle, c.name, c)
	}

	return f
}

func mustAddComponent(t *testing.T, l *lifecycle.Lifecycle, name string, c lifecycle.Component) {
	t.Helper()

	if err := l.AddComponent(name, c); err != nil {
		t.Fatalf("AddComponent(%q) error = %v, want nil", name, err)
	}
}

func assertEvents(t *testing.T, got, want []string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestAddComponent(t *testing.T) {
	t.Parallel()

	type input struct {
		name      string
		component lifecycle.Component
	}

	tests := []struct {
		name  string
		setup func(t *testing.T, l *lifecycle.Lifecycle)
		input input
		want  error
	}{
		{
			name:  "success",
			input: input{"PostgreSQL", &fakeComponent{}},
			want:  nil,
		},
		{
			name:  "empty name",
			input: input{"", &fakeComponent{}},
			want:  lifecycle.ErrComponentNameIsEmpty,
		},
		{
			name:  "nil component",
			input: input{"Kafka", nil},
			want:  lifecycle.ErrComponentIsNil,
		},
		{
			name: "duplicate component name",
			setup: func(t *testing.T, l *lifecycle.Lifecycle) {
				mustAddComponent(t, l, "Redis", &fakeComponent{})
			},
			input: input{"Redis", &fakeComponent{}},
			want:  lifecycle.ErrComponentNameAlreadyExists,
		},
		{
			name: "add after startup",
			setup: func(t *testing.T, l *lifecycle.Lifecycle) {
				_, _ = l.Startup(context.Background())
			},
			input: input{"ClickHouse", &fakeComponent{}},
			want:  lifecycle.ErrLifecycleIsSealed,
		},
		{
			name: "add after shutdown",
			setup: func(t *testing.T, l *lifecycle.Lifecycle) {
				_, _ = l.Shutdown(context.Background())
			},
			input: input{"RabbitMQ", &fakeComponent{}},
			want:  lifecycle.ErrLifecycleIsSealed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := lifecycle.New()

			if tt.setup != nil {
				tt.setup(t, l)
			}

			got := l.AddComponent(tt.input.name, tt.input.component)

			if !errors.Is(got, tt.want) {
				t.Fatalf("AddComponent() error = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddComponent_Concurrent(t *testing.T) {
	t.Parallel()

	const n = 50

	l := lifecycle.New()
	errs := make([]error, n)

	var wg sync.WaitGroup

	for i := range n {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Имена уникальны, поэтому все добавления должны пройти успешно
			errs[i] = l.AddComponent(string(rune('A'+i)), &fakeComponent{})
		}()
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("AddComponent() #%d error = %v, want nil", i, err)
		}
	}

	reports, err := l.Startup(context.Background())
	if err != nil {
		t.Fatalf("Startup() error = %v, want nil", err)
	}

	if len(reports) != n {
		t.Fatalf("len(reports) = %d, want %d", len(reports), n)
	}
}

func TestStartup(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	reports, err := f.lifecycle.Startup(context.Background())
	if err != nil {
		t.Fatalf("Startup() error = %v, want nil", err)
	}

	// При старте компоненты запускаются в порядке регистрации (FIFO)
	wantOrder := []string{"PostgreSQL", "Kafka", "Redis"}

	if len(reports) != len(wantOrder) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(wantOrder))
	}

	for i, want := range wantOrder {
		if got := reports[i].ComponentName; got != want {
			t.Errorf("reports[%d].ComponentName = %q, want %q", i, got, want)
		}
	}

	assertEvents(t, *f.events, []string{"startup:PostgreSQL", "startup:Kafka", "startup:Redis"})

	for _, c := range f.all() {
		c.assertStartupCalled(t, 1)
		c.assertShutdownCalled(t, 0)
	}
}

func TestStartup_WithErrors(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	pgErr := errors.New("PostgreSQL startup error")
	redisErr := errors.New("Redis startup error")

	f.postgres.startupErr = pgErr
	f.redis.startupErr = redisErr

	reports, err := f.lifecycle.Startup(context.Background())

	if len(reports) != len(f.all()) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(f.all()))
	}

	if !errors.Is(err, pgErr) || !errors.Is(err, redisErr) {
		t.Fatalf("Startup() error = %v, want joined %v and %v", err, pgErr, redisErr)
	}

	// Ошибка одного компонента не должна останавливать запуск остальных
	for _, c := range f.all() {
		c.assertStartupCalled(t, 1)
	}

	if !errors.Is(reports[0].Err, pgErr) {
		t.Errorf("reports[0].Err = %v, want %v", reports[0].Err, pgErr)
	}

	if reports[1].Err != nil {
		t.Errorf("reports[1].Err = %v, want nil", reports[1].Err)
	}

	if !errors.Is(reports[2].Err, redisErr) {
		t.Errorf("reports[2].Err = %v, want %v", reports[2].Err, redisErr)
	}
}

func TestStartup_WithContextCancel(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reports, err := f.lifecycle.Startup(ctx)

	if len(reports) != len(f.all()) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(f.all()))
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Startup() error = %v, want %v", err, context.Canceled)
	}

	// Контекст отменен до старта, поэтому все компоненты должны быть пропущены
	for i, r := range reports {
		if !errors.Is(r.Err, context.Canceled) {
			t.Errorf("reports[%d].Err = %v, want %v", i, r.Err, context.Canceled)
		}
	}

	for _, c := range f.all() {
		c.assertStartupCalled(t, 0)
	}
}

func TestStartup_WithContextTimeout(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	// PostgreSQL стартует мгновенно, Kafka зависает до дедлайна, а Redis будет пропущен
	f.kafka.blockOnStartup = true

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	reports, err := f.lifecycle.Startup(ctx)

	if len(reports) != len(f.all()) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(f.all()))
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Startup() error = %v, want %v", err, context.DeadlineExceeded)
	}

	if reports[0].Err != nil {
		t.Errorf("reports[0].Err = %v, want nil", reports[0].Err)
	}

	for _, i := range []int{1, 2} {
		if !errors.Is(reports[i].Err, context.DeadlineExceeded) {
			t.Errorf("reports[%d].Err = %v, want %v", i, reports[i].Err, context.DeadlineExceeded)
		}
	}

	f.postgres.assertStartupCalled(t, 1)
	f.kafka.assertStartupCalled(t, 1)
	f.redis.assertStartupCalled(t, 0)
}

func TestShutdown(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	reports, err := f.lifecycle.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown() error = %v, want nil", err)
	}

	// При остановке компоненты останавливаются в обратном порядке (LIFO)
	wantOrder := []string{"Redis", "Kafka", "PostgreSQL"}

	if len(reports) != len(wantOrder) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(wantOrder))
	}

	for i, want := range wantOrder {
		if got := reports[i].ComponentName; got != want {
			t.Errorf("reports[%d].ComponentName = %q, want %q", i, got, want)
		}
	}

	assertEvents(t, *f.events, []string{"shutdown:Redis", "shutdown:Kafka", "shutdown:PostgreSQL"})

	for _, c := range f.all() {
		c.assertStartupCalled(t, 0)
		c.assertShutdownCalled(t, 1)
	}
}

func TestShutdown_WithErrors(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	pgErr := errors.New("PostgreSQL shutdown error")
	redisErr := errors.New("Redis shutdown error")

	f.postgres.shutdownErr = pgErr
	f.redis.shutdownErr = redisErr

	reports, err := f.lifecycle.Shutdown(context.Background())

	if len(reports) != len(f.all()) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(f.all()))
	}

	if !errors.Is(err, pgErr) || !errors.Is(err, redisErr) {
		t.Fatalf("Shutdown() error = %v, want joined %v and %v", err, pgErr, redisErr)
	}

	// Ошибка одного компонента не должна мешать остановке остальных
	for _, c := range f.all() {
		c.assertShutdownCalled(t, 1)
	}

	// Порядок отчетов обратный: Redis, Kafka, PostgreSQL
	if !errors.Is(reports[0].Err, redisErr) {
		t.Errorf("reports[0].Err = %v, want %v", reports[0].Err, redisErr)
	}

	if reports[1].Err != nil {
		t.Errorf("reports[1].Err = %v, want nil", reports[1].Err)
	}

	if !errors.Is(reports[2].Err, pgErr) {
		t.Errorf("reports[2].Err = %v, want %v", reports[2].Err, pgErr)
	}
}

func TestShutdown_WithContextCancel(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reports, err := f.lifecycle.Shutdown(ctx)

	if len(reports) != len(f.all()) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(f.all()))
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown() error = %v, want %v", err, context.Canceled)
	}

	for i, r := range reports {
		if !errors.Is(r.Err, context.Canceled) {
			t.Errorf("reports[%d].Err = %v, want %v", i, r.Err, context.Canceled)
		}
	}

	for _, c := range f.all() {
		c.assertShutdownCalled(t, 0)
	}
}

func TestShutdown_WithContextTimeout(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	// Redis останавливается первым и мгновенно, Kafka зависает до дедлайна, а PostgreSQL будет пропущен
	f.kafka.blockOnShutdown = true

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	reports, err := f.lifecycle.Shutdown(ctx)

	if len(reports) != len(f.all()) {
		t.Fatalf("len(reports) = %d, want %d", len(reports), len(f.all()))
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want %v", err, context.DeadlineExceeded)
	}

	if reports[0].Err != nil {
		t.Errorf("reports[0].Err = %v, want nil", reports[0].Err)
	}

	for _, i := range []int{1, 2} {
		if !errors.Is(reports[i].Err, context.DeadlineExceeded) {
			t.Errorf("reports[%d].Err = %v, want %v", i, reports[i].Err, context.DeadlineExceeded)
		}
	}

	f.redis.assertShutdownCalled(t, 1)
	f.kafka.assertShutdownCalled(t, 1)
	f.postgres.assertShutdownCalled(t, 0)
}
