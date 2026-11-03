// Command example демонстрирует использование пакета lifecycle:
// регистрацию компонентов, запуск, обработку сигналов и корректную остановку.
//
// Запуск:
//
//	go run ./example
//
// Для демонстрации обработки ошибок запуска:
//
//	EXAMPLE_FAIL_STARTUP=1 go run ./example
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RomanDeveloperGit/go-lifecycle/pkg/lifecycle"
)

func main() {
	log.SetFlags(0)

	// Контекст, который отменяется по Ctrl+C или SIGTERM.
	appCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	app := lifecycle.New()

	components := []struct {
		name      string
		component lifecycle.Component
	}{
		{
			name:      "postgres",
			component: &postgres{dsn: "postgres://demo:demo@localhost:5432/demo"},
		},
		{
			name: "cache",
			component: &cache{
				addr: "127.0.0.1:6379",
				// Позволяет показать, как lifecycle собирает ошибки запуска.
				failStartup: os.Getenv("EXAMPLE_FAIL_STARTUP") == "1",
			},
		},
		{
			name:      "http",
			component: &httpServer{addr: "127.0.0.1:0"},
		},
	}

	// Компоненты запускаются в порядке регистрации, а останавливаются в обратном.
	for _, c := range components {
		if err := app.AddComponent(c.name, c.component); err != nil {
			log.Fatalf("add component %q: %v", c.name, err)
		}
	}

	log.Print("starting application")

	startupCtx, cancelStartup := context.WithTimeout(appCtx, 5*time.Second)
	startupErr := start(app, startupCtx)
	cancelStartup()

	if startupErr != nil {
		// Ошибка одного компонента не прерывает запуск остальных,
		// поэтому остановить приложение всё равно нужно.
		log.Printf("startup finished with errors: %v", startupErr)
	} else {
		log.Print("application is running, press Ctrl+C to stop")
		<-appCtx.Done()
		log.Print("shutdown signal received")
	}

	log.Print("shutting down application")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	shutdownErr := stop(app, shutdownCtx)
	cancelShutdown()

	if shutdownErr != nil {
		log.Fatalf("shutdown finished with errors: %v", shutdownErr)
	}

	if startupErr != nil {
		os.Exit(1)
	}

	log.Print("application stopped")
}
