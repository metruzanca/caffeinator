# Development

## Project structure

```text
.
├── agent/                     # AI engine, no Discord or project-internal imports
│   ├── agent.go               # Prompt assembly, memory injection, model calls
│   ├── openrouter.go          # OpenRouter chat client
│   ├── image.go               # OpenRouter image client
│   ├── memory.go              # Markdown memory store
│   ├── memestore.go           # Meme memory, relevance, reactions, retirement
│   ├── extractor.go           # Fact extraction
│   └── message.go             # User, turn, attachment, and image types
├── docs/                      # Feature and configuration docs
├── internal/
│   ├── config/                # Viper, dotenv, branding, logging
│   ├── memes/                 # Reddit scraper, image analysis, runner, cron
│   ├── ratelimit/             # Shared token bucket limiter
│   ├── reminders/             # Reminder store, parser, recurrence, scheduler
│   ├── server/                # Discord adapter and bot lifecycle
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

The suite covers agent prompt assembly, mention detection, `/ask`, memory storage and extraction, meme scraping and retrieval, image generation request and response handling, reminders and recurrence, response chunking, rate limiting, and the OpenRouter clients. Tests use `httptest` and never call live APIs.

`agent/` is the AI engine and must stay free of Discord types. `internal/server/` wires it to discordgo.

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
