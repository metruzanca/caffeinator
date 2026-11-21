# Development

## Project structure

```text
.
├── cmd/                       # CLI commands (run, invite, command clear)
├── docs/                      # Feature and configuration docs
├── internal/
│   ├── config/                # Viper, dotenv, branding, logging
│   ├── imagegen/              # OpenRouter image generation client
│   ├── llm/                   # OpenRouter chat client
│   ├── memory/                # Markdown memory store and extraction
│   ├── ratelimit/             # Shared token bucket limiter
│   ├── reminders/             # Reminder store, parser, recurrence, scheduler
│   ├── server/                # Bot lifecycle and chat pipeline
│   │   └── commands/          # Slash commands
│   ├── util/                  # Small helpers
│   └── version/               # Build metadata
├── .github/workflows/ci.yml   # Format, vet, and test
├── Dockerfile
├── go.mod
└── main.go
```

## Build and test

```bash
go build ./...
go test ./...
go vet ./...
```

The suite covers link rewriting, context assembly, mention detection, `/ask`, memory storage and extraction, image generation request and response handling, reminders and recurrence, response chunking, rate limiting, and the OpenRouter clients. Tests use `httptest` and never call live APIs.

## Local run

```bash
go run .
```

Live reload works with [air](https://github.com/air-verse/air):

```bash
air
```

## Adding a slash command

1. Create a file in `internal/server/commands/`.
2. Define a `Command` with `ApplicationCommand` and a `Handler`.
3. Register it in `init()` with `registerCommand(...)`.
4. Use `{bot}` in a description when it should include `BOT_NAME`. It is replaced at registration time.
5. Run `go test ./...` and `go vet ./...`.

## Logging

Logs go to stdout. Locally they use charmbracelet's text format. On Railway they switch to JSON automatically.

Use `github.com/charmbracelet/log` with key-value pairs:

```go
log.Info("chat request", "channel", channelID, "user", userID)
log.Error("request failed", "err", err, "channel", channelID)
```

Do not log message contents or API keys.
