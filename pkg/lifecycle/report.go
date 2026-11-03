package lifecycle

import (
	"errors"
	"fmt"
	"time"
)

type phase string

const (
	phaseStartup  phase = "startup"
	phaseShutdown phase = "shutdown"
)

type report struct {
	ComponentName string
	Phase         phase
	Duration      time.Duration
	Skipped       bool
	Err           error
}

func newReport(componentName string, phase phase, duration time.Duration, skipped bool, err error) report {
	return report{
		ComponentName: componentName,
		Phase:         phase,
		Duration:      duration,
		Skipped:       skipped,
		Err:           err,
	}
}

func (r report) OK() bool {
	return r.Err == nil
}

type reportSlice []report

func (r reportSlice) err() error {
	errs := make([]error, 0, len(r))

	for _, rep := range r {
		if rep.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", rep.ComponentName, rep.Err))
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return fmt.Errorf("reports errors:\n %w", errors.Join(errs...))
}
