# Project Conventions and Architecture Guide

## Quick Summary for AI Agents

**Critical rules**

- **MUST**: Use `github.com/charmbracelet/log` for logging, not the standard library `log`.
- **MUST**: Use structured logging with key-value pairs: `log.Info("message", "key", value)`.
- **MUST**: Respond to every Discord interaction exactly once.
- **MUST**: Register slash commands from an `init()` function with `registerCommand()`.
- **MUST**: Use "server" instead of "guild" in user-facing text.
- **MUST**: Never log message contents or API keys.
- **MUST NOT**: Add a database layer without an explicit product decision. The bot is stateless right now.

**Key files**

- Commands: `internal/server/commands/*.go`
- Command registration: `internal/server/commands/commands.go`
- Bot lifecycle: `internal/server/bot.go`
- Mention chat: `internal/server/chat.go`
- OpenRouter client: `internal/llm/openrouter.go`
- Rate limiter: `internal/ratelimit/ratelimit.go`
- Configuration: `internal/config/config.go` and `cmd/root.go`
- CLI: `cmd/*.go`

**Build and test**

```bash
go build ./...
go vet ./...
go test ./...
```

---

## Architecture

The bot is a stateless Discord client. There is no database, ORM, migration tool, or `go generate` step.

Two features exist:

1. `/link` rewrites social media URLs for embed-friendly hosts.
2. Mention chat sends recent channel context to OpenRouter and posts the response.

`cmd/root.go` starts the bot and handles shutdown. `internal/server.NewBot` creates the Discord session and sets the required intents. `Bot.Start` connects, registers the chat handler, installs slash commands, and handles newly joined servers.

---

## Logging

Use `github.com/charmbracelet/log`.

Pattern:

```go
log.Info("command executed", "command", name, "user", userID, "channel", channelID)
log.Error("cannot send response", "err", err, "channel", channelID)
```

Rules:

- Error logs MUST include an `"err"` key.
- Command logs SHOULD include `"command"`, `"user"`, and `"channel"`.
- Chat logs SHOULD include `"guild"`, `"channel"`, and `"user"`, plus latency and token usage.
- NEVER log message text, prompts, or API keys.
- Use `Debug` for noise, `Info` for normal operation, `Warn` for recoverable problems, `Error` for failures, and `Fatal` only during startup.

---

## Slash Commands

Location: `internal/server/commands/`

Command type:

```go
type Command struct {
    discordgo.ApplicationCommand
    Handler func(s *discordgo.Session, i *discordgo.InteractionCreate)
}
```

Registration:

```go
var ExampleCmd = &Command{
    ApplicationCommand: discordgo.ApplicationCommand{
        Name:        "example",
        Description: "Example command",
    },
    Handler: func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        // Respond exactly once.
    },
}

func init() {
    registerCommand(ExampleCmd)
}
```

Lifecycle:

- Commands are registered per guild.
- `clearUnregisteredCommands` deletes commands that exist in Discord but not in the current build.
- Commands are rate limited per user by `commandRateLimiter`.

Handler rules:

- MUST use `s.InteractionRespond` to respond.
- MUST respond exactly once. If a response fails, log it.
- MUST use user-friendly messages and say "server", not "guild".
- MUST log failures before responding.

---

## Link Rewriting

Location: `internal/server/commands/link.go`

`RewriteLink` is a pure function. Keep it that way so it stays easy to test.

Rules:

- Rewrite hosts only from the explicit allowlist.
- Force HTTPS.
- Remove query strings and fragments.
- Preserve the path.
- Do not fetch the URL. This avoids SSRF.

Add new hosts to the switch in `RewriteLink` and add table-driven test cases.

---

## Mention Chat

Location: `internal/server/chat.go`

Trigger conditions:

- The bot is mentioned, or
- The message is a reply to a bot message, even without an explicit mention.

Ignore:

- The bot's own messages.
- Messages from any other bot.
- DMs and messages outside guilds.
- Messages that neither mention the bot nor reply to it.

Context rules:

- Plain mention: last `CHAT_CONTEXT_MESSAGES` messages before the mention.
- Reply: the referenced message plus `CHAT_CONTEXT_MESSAGES - 1` messages before it.
- Fall back to channel history if the referenced message is unavailable.
- Include messages in chronological order.
- Bot messages become `assistant` turns.
- Other messages become `user` turns prefixed with the author's display name.
- No system prompt is sent.

Response rules:

- Reply to the triggering message.
- Disable all mentions with `AllowedMentions` and `RepliedUser: false`.
- Split responses over Discord's 2000-character limit, up to `CHAT_MAX_CHUNKS`.
- Use a typing indicator while waiting.
- Rate limit per user and cap concurrent requests.

---

## LLM Client

Location: `internal/llm/openrouter.go`

- OpenAI-compatible `POST /chat/completions` at OpenRouter.
- No system message is added by the client.
- Retry once on network errors, `429`, and `5xx`.
- Never log the API key.
- Return clear errors that include status and a short response body.
- Keep the client independent of Discord types so it can be tested with `httptest`.

---

## Configuration

Configuration uses Viper and dotenv.

- Defaults and CLI flags live in `cmd/root.go`.
- Environment variables override defaults.
- `OPENROUTER_API_KEY` is environment-only and should never be added as a flag.
- `.env` is gitignored. Keep `.env.example` current.
- If the OpenRouter key is empty, mention chat is disabled and `/link` still works.

---

## Testing

- Prefer pure functions for logic that can be tested without Discord.
- Use interfaces for Discord and LLM dependencies in `chat.go`.
- Use `httptest` for the OpenRouter client.
- Do not call live Discord or OpenRouter APIs in tests.
- Add table-driven cases for link rewriting.
- Run `go test ./...`, `go vet ./...`, and `go build ./...` before finishing.

---

## Go Style

- Keep packages small and focused.
- Wrap errors with context using `%w`.
- Use `context.Context` for network calls.
- Avoid hidden global state except the command registry and rate limiter.
- Use the built-in `min` instead of a custom helper.
- Prefer explicit code over clever abstractions.
