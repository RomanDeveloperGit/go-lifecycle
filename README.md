# go-lifecycle

Небольшая библиотека для управления жизненным циклом компонентов Go-приложения. Она запускает
компоненты в порядке регистрации, останавливает их в обратном порядке, собирает отчёты о каждой
операции и объединяет ошибки в одну.

## Возможности

- Запуск компонентов в порядке добавления, остановка — в обратном порядке (LIFO).
- Ошибка одного компонента не прерывает запуск или остановку остальных.
- Отчёты по каждому компоненту: имя, фаза, длительность, признак пропуска и ошибка.
- Поддержка `context.Context`: при отмене контекста оставшиеся компоненты пропускаются.
- Защита от дубликатов имён, пустых имён и `nil`-компонентов.
- После первого запуска или остановки список компонентов фиксируется.
- Потокобезопасность: все операции защищены мьютексом.

## Установка

```bash
go get github.com/RomanDeveloperGit/go-lifecycle
```

Требуется Go 1.27.2 или новее.

## Быстрый старт

Компонентом является любой тип, реализующий интерфейс `lifecycle.Component`:

```go
type Component interface {
	Startup(ctx context.Context) error
	Shutdown(ctx context.Context) error
}
```

Пример:

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/RomanDeveloperGit/go-lifecycle/pkg/lifecycle"
)

type server struct{}

func (s *server) Startup(ctx context.Context) error  { return nil }
func (s *server) Shutdown(ctx context.Context) error { return nil }

func main() {
	app := lifecycle.New()

	if err := app.AddComponent("server", &server{}); err != nil {
		log.Fatal(err)
	}

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStartup()

	reports, err := app.Startup(startupCtx)

	for _, r := range reports {
		log.Printf(
			"%s: phase=%s duration=%s skipped=%t err=%v",
			r.ComponentName, r.Phase, r.Duration, r.Skipped, r.Err,
		)
	}

	if err != nil {
		log.Fatal(err)
	}

	// ... работа приложения ...

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if _, err := app.Shutdown(shutdownCtx); err != nil {
		log.Fatal(err)
	}
}
```

## Поведение

### Порядок вызовов

Компоненты запускаются в порядке регистрации, а останавливаются в обратном порядке. Например,
если добавить `postgres`, `cache` и `http`, то `Startup` вызовет их в порядке
`postgres → cache → http`, а `Shutdown` — в порядке `http → cache → postgres`.

### Ошибки

- Если компонент вернул ошибку, lifecycle продолжает работу с остальными компонентами.
- `Startup` и `Shutdown` возвращают объединённую ошибку, поэтому исходные ошибки можно искать
  через `errors.Is`.
- Ошибка конкретного компонента также доступна в его отчёте (`report.Err`).

### Контекст

Перед вызовом каждого компонента проверяется `ctx.Done()`. Если контекст уже отменён или истёк
дедлайн, оставшиеся компоненты не вызываются, а в отчёте для них выставляется `Skipped = true`
и `Err = ctx.Err()`. Уже запущенные компоненты не откатываются автоматически.

Компонент должен сам учитывать контекст внутри `Startup`/`Shutdown`, если операция может
занимать длительное время.

### Отчёты

`Startup` и `Shutdown` возвращают срез отчётов. У каждого отчёта доступны поля:

| Поле            | Описание                                      |
| --------------- | --------------------------------------------- |
| `ComponentName` | Имя компонента                                |
| `Phase`         | Фаза: `startup` или `shutdown`                |
| `Duration`      | Длительность операции                         |
| `Skipped`       | `true`, если компонент пропущен из-за контекста |
| `Err`           | Ошибка компонента или `nil`                   |

Метод `OK()` возвращает `true`, если ошибки нет.

### Фиксация списка компонентов

После первого вызова `Startup` или `Shutdown` lifecycle «запечатывается»: `AddComponent`
начинает возвращать `ErrLifecycleIsSealed`. Это защищает от изменения состава компонентов
во время работы приложения.

## API

| Функция / метод                                                     | Описание                                      |
| ------------------------------------------------------------------- | --------------------------------------------- |
| `lifecycle.New() *lifecycle.Lifecycle`                              | Создаёт новый lifecycle                       |
| `(*Lifecycle).AddComponent(name string, c Component) error`         | Добавляет компонент с уникальным именем       |
| `(*Lifecycle).Startup(ctx context.Context) (reports, error)`        | Запускает все компоненты                      |
| `(*Lifecycle).Shutdown(ctx context.Context) (reports, error)`       | Останавливает все компоненты в обратном порядке |

Сентинел-ошибки:

- `lifecycle.ErrComponentNameIsEmpty` — пустое имя компонента;
- `lifecycle.ErrComponentNameAlreadyExists` — компонент с таким именем уже добавлен;
- `lifecycle.ErrComponentIsNil` — передан `nil`-компонент;
- `lifecycle.ErrLifecycleIsSealed` — попытка добавить компонент после запуска или остановки.

## Пример

В директории [`example`](./example) находится запускаемое приложение с компонентами PostgreSQL,
Redis-подобного кэша и HTTP-сервера:

```bash
go run ./example
```

Приложение запустится и будет ждать `Ctrl+C`, после чего корректно остановит компоненты.

Чтобы посмотреть, как обрабатываются ошибки запуска, запустите:

```bash
EXAMPLE_FAIL_STARTUP=1 go run ./example
```

В этом режиме кэш вернёт ошибку, но lifecycle продолжит запуск остальных компонентов, соберёт
ошибки в отчёт и затем остановит приложение.

## Разработка

```bash
go test ./...
task lint
task format
```
