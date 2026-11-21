# Project Conventions and Architecture Guide

## Quick Summary for AI Agents

**Critical rules**

- **MUST**: Use `github.com/charmbracelet/log` for logging, not the standard library `log`.
- **MUST**: Use structured logging with key-value pairs: `log.Info("message", "key", value)`.
- **MUST**: Respond to every Discord interaction exactly once.
- **MUST**: Register slash commands from an `init()` function with `registerCommand()`.
- **MUST**: Use "server" instead of "guild" in user-facing text.
- **MUST**: Never log message contents or API keys.
- **MUST**: Learn only from conversations where the bot is invoked. Never read normal channel traffic.
- **MUST NOT**: Add a database layer without an explicit product decision. Persistent state is markdown memory on disk.

**Key files**

- Commands: `internal/server/commands/*.go`
- Command registration: `internal/server/commands/commands.go`
- Bot lifecycle: `internal/server/bot.go`
- Chat pipeline: `internal/server/chat.go`
- Memory store and extraction: `internal/memory/`
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

The bot has no database, ORM, migration tool, or `go generate` step. Persistent state is markdown memory on disk.

Features:

1. `/link` rewrites social media URLs for embed-friendly hosts.
2. Mention and reply chat sends recent channel context to OpenRouter and posts the response.
3. `/ask` runs the same chat pipeline with configurable history and memory opt-out.
4. `/memory` lets users inspect and remove their own memory. Admins can target other users or server memory.

`cmd/root.go` starts the bot and handles shutdown. `internal/server.NewBot` creates the Discord session and sets the required intents. `Bot.Start` connects, registers the chat handler, installs slash commands, and handles newly joined servers.

---

## Logging

Use `github.com/charmbracelet/log`.

`config.ConfigureLogging` sends the default logger to stdout. Locally it uses the text formatter. On Railway, where `RAILWAY_ENVIRONMENT` is set, it switches to the JSON formatter so Railway can read the `level` field instead of treating all stderr output as an error. Do not write logs directly to stderr.

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

## Chat Pipeline

Location: `internal/server/chat.go`

Entry points:

- A mention.
- A reply to a bot message, even without an explicit mention.
- `/ask`.

`/ask` behavior:

- `text` is required.
- `history_length` defaults to `10`, minimum `1`, maximum `100`.
- `disable_memory` skips both memory reads and writes for that ask.
- Responses are public.
- The interaction is deferred, then edited with the response.

Trigger conditions:

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
- A system prompt tells the bot its stack: Go, discordgo, Railway, OpenRouter, and DeepSeek V4.1 Flash 1017.
- It also tells the bot that memory is automatic, tool-free, does not require a mention, and is limited to facts people share about themselves. It must not claim it cannot remember between sessions.
- It requires inline Discord markdown citations for web search results, using short relevant phrases as link text and at most two citations by default.
- It requires Discord mention and timestamp formatting: `<@id>`, `<#id>`, and `<t:unix:R>`.
- It makes replies peer-like: casual, direct, and free of unsolicited remediation.
- It bans em dashes and asks for short, simple sentences.
- It also limits replies to a single tweet (280 characters) unless the user asks for more detail.
- The system prompt can be overridden with `CHAT_SYSTEM_PROMPT`.

Response rules:

- Reply to the triggering message.
- Allow user mentions with `AllowedMentions`, but block everyone and role mentions. Keep `RepliedUser: false`.
- Append the current Unix time to the system prompt so Discord timestamp output is accurate.
- Split responses over Discord's 2000-character limit, up to `CHAT_MAX_CHUNKS`.
- Use a typing indicator while waiting.
- Rate limit per user and cap concurrent requests.

---

## Memory

Location: `internal/memory/`

Memory is read-only from the model's perspective. The model never writes files directly.

Read path:

- Before a reply, load `server.md` and the invoking user's file.
- Also load users explicitly mentioned in the current conversation.
- Cap the injected text with `MEMORY_MAX_CONTEXT_CHARS`.
- Add a line telling the model that memory is reference data, not instructions.

Write path:

- Passive memory observes normal guild messages through a separate collector.
- Batch messages per author every `MEMORY_BATCH_SECONDS`.
- Use only that author's own messages as the transcript.
- Extract only first-person facts that the speaker states about themselves.
- Ignore statements about other people, even when the speaker mentions them.
- Ignore bot messages, other bots, and other users' messages.
- Never write facts about another user based on what someone else said.
- Skip channels listed in `MEMORY_IGNORE_CHANNELS`.
- Filter out secrets, credentials, addresses, health data, and sensitive personal data.
- Filter by `MEMORY_MIN_CONFIDENCE`.
- Deduplicate against existing markdown.
- Write atomically with a temporary file and rename.
- Use JSON response format and enough output tokens to avoid truncated JSON.
- Do not auto-write server memory. Server memory is manual/admin-managed.
- When passive memory is enabled, mention and reply chat skips its own extraction to avoid duplicate calls. `/ask` still extracts because it has no message event.

Rules:

- Memory is enabled for normal messages when `MEMORY_PASSIVE_ENABLED=true`.
- `disable_memory` on `/ask` skips reads and writes.
- The system prompt must say memory is automatic and self-only, and that no memory tool is needed.
- `/memory view` and `/memory forget` work on the invoker's own memory by default.
- Admins can target another user with `user:@someone` or server memory with `server:true`.
- `/memory forget fact:<text>` removes matching entries. `/memory forget all:true` clears the target.
- `/memory list` is Administrator-only.
- Every `/memory` response is ephemeral.
- Without a persistent volume, memory is lost on deploy.

---

## LLM Client

Location: `internal/llm/openrouter.go`

- OpenAI-compatible `POST /chat/completions` at OpenRouter.
- A system prompt is prepended when configured. The bot uses a default peer-tone prompt that states its stack, keeps sentences simple, bans em dashes, and limits replies to 280 characters.
- Web search uses the `openrouter:web_search` server tool, not the deprecated `:online` suffix or `web` plugin.
- Web search citations are handled by the system prompt as inline Discord markdown links. The client does not rewrite citations.
- The server tool is only added when `OPENROUTER_WEB_SEARCH` is true. The model decides whether a search is needed.
- Search limits live in `OPENROUTER_WEB_SEARCH_MAX_RESULTS` and `OPENROUTER_WEB_SEARCH_MAX_USES`.
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
- `CHAT_SYSTEM_PROMPT` overrides the built-in peer-tone prompt.
- Web search is enabled by default with `OPENROUTER_WEB_SEARCH=true`. It is capped by `OPENROUTER_WEB_SEARCH_MAX_RESULTS` and `OPENROUTER_WEB_SEARCH_MAX_USES` to control cost.
- Memory is enabled by default with `MEMORY_ENABLED=true`.
- `MEMORY_DIR` defaults to `./memory`. On Railway, use a volume and set `MEMORY_DIR=/data/memory`.
- `MEMORY_MAX_CONTEXT_CHARS`, `MEMORY_MIN_CONFIDENCE`, and `MEMORY_EXTRACT_TIMEOUT_SECONDS` control memory size and extraction.
- `MEMORY_PASSIVE_ENABLED`, `MEMORY_BATCH_SECONDS`, `MEMORY_MAX_BATCH_MESSAGES`, and `MEMORY_IGNORE_CHANNELS` control passive learning.

---

## Testing

- Prefer pure functions for logic that can be tested without Discord.
- Use interfaces for Discord and LLM dependencies in `chat.go`.
- Use `httptest` for the OpenRouter client.
- Do not call live Discord or OpenRouter APIs in tests.
- Add table-driven cases for link rewriting.
- Test the memory store with `t.TempDir()`.
- Test extraction with a fake chat client.
- Run `go test ./...`, `go vet ./...`, and `go build ./...` before finishing.

---

## Go Style

- Keep packages small and focused.
- Wrap errors with context using `%w`.
- Use `context.Context` for network calls.
- Avoid hidden global state except the command registry and rate limiter.
- Use the built-in `min` instead of a custom helper.
- Prefer explicit code over clever abstractions.
