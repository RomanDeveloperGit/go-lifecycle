package main

import (
	"fmt"
	"time"
)

// printReport печатает одну строку отчёта lifecycle.
func printReport(name, phase string, duration time.Duration, skipped, ok bool, err error) {
	fmt.Printf(
		"  %-9s phase=%-8s status=%-8s duration=%-8s",
		name,
		phase,
		reportStatus(skipped, ok),
		duration.Round(time.Millisecond),
	)

	if err != nil {
		fmt.Printf(" err=%v", err)
	}

	fmt.Println()
}

func reportStatus(skipped, ok bool) string {
	switch {
	case skipped:
		return "skipped"
	case !ok:
		return "failed"
	default:
		return "ok"
	}
}
