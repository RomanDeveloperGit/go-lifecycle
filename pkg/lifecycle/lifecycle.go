package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrLifecycleIsSealed = errors.New("lifecycle is sealed")

	ErrComponentNameIsEmpty       = errors.New("component name is empty")
	ErrComponentNameAlreadyExists = errors.New("component name already exists")
	ErrComponentIsNil             = errors.New("component is nil")
)

type Lifecycle struct {
	// После первого запуска переводится в true, чтобы запрещать добавлять новые компоненты; MVP-версия
	sealed bool

	// Список компонентов
	components []*component

	// Для обеспечения правила: в 1 единицу времени выполняется только 1 операция
	mu sync.Mutex
}

func New() *Lifecycle {
	return &Lifecycle{
		sealed:     false,
		components: make([]*component, 0),
		mu:         sync.Mutex{},
	}
}

type Component interface {
	Startup(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

func (l *Lifecycle) AddComponent(name string, component Component) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.sealed {
		return ErrLifecycleIsSealed
	}

	if name == "" {
		return ErrComponentNameIsEmpty
	}

	if component == nil {
		return ErrComponentIsNil
	}

	for _, c := range l.components {
		if c.name == name {
			return fmt.Errorf("%w: %s", ErrComponentNameAlreadyExists, name)
		}
	}

	l.components = append(l.components, newComponent(name, component.Startup, component.Shutdown))

	return nil
}

func (l *Lifecycle) Startup(ctx context.Context) ([]report, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.sealed {
		l.sealed = true
	}

	reports := make(reportSlice, 0, len(l.components))

	for _, c := range l.components {
		select {
		case <-ctx.Done():
			reports = append(reports, newReport(c.name, phaseStartup, 0, true, ctx.Err()))
		default:
			beforeStartupTime := time.Now()
			err := c.startup(ctx)

			reports = append(
				reports,
				newReport(c.name, phaseStartup, time.Since(beforeStartupTime), false, err),
			)
		}
	}

	return reports, errors.Join(ctx.Err(), reports.err())
}

func (l *Lifecycle) Shutdown(ctx context.Context) ([]report, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.sealed {
		l.sealed = true
	}

	reports := make(reportSlice, 0, len(l.components))

	for i := len(l.components) - 1; i >= 0; i-- {
		select {
		case <-ctx.Done():
			reports = append(
				reports,
				newReport(l.components[i].name, phaseShutdown, 0, true, ctx.Err()),
			)
		default:
			beforeShutdownTime := time.Now()
			err := l.components[i].shutdown(ctx)

			reports = append(
				reports,
				newReport(
					l.components[i].name,
					phaseShutdown,
					time.Since(beforeShutdownTime),
					false,
					err,
				),
			)
		}
	}

	return reports, errors.Join(ctx.Err(), reports.err())
}
