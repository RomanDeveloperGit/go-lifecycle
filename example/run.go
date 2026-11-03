package main

import (
	"context"
	"fmt"

	"github.com/RomanDeveloperGit/go-lifecycle/pkg/lifecycle"
)

// start запускает приложение и печатает отчёты по каждому компоненту.
func start(app *lifecycle.Lifecycle, ctx context.Context) error {
	reports, err := app.Startup(ctx)

	fmt.Println("\nstartup reports:")

	for _, r := range reports {
		printReport(r.ComponentName, string(r.Phase), r.Duration, r.Skipped, r.OK(), r.Err)
	}

	return err
}

// stop останавливает приложение и печатает отчёты по каждому компоненту.
func stop(app *lifecycle.Lifecycle, ctx context.Context) error {
	reports, err := app.Shutdown(ctx)

	fmt.Println("\nshutdown reports:")

	for _, r := range reports {
		printReport(r.ComponentName, string(r.Phase), r.Duration, r.Skipped, r.OK(), r.Err)
	}

	return err
}
