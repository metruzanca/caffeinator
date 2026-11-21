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
- Reminder store, parser, scheduler: `internal/reminders/`
- Reminder Discord integration: `internal/server/reminders.go`
- OpenRouter chat client: `internal/llm/openrouter.go`
- OpenRouter image client: `internal/imagegen/openrouter.go`
- Rate limiter: `internal/ratelimit/ratelimit.go`
- Configuration: `internal/config/config.go` and `cmd/root.go`
- CLI: `cmd/*.go`
- Docs: `README.md` and `docs/`. Keep the README short and put details in `docs/`.

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

1. Mention and reply chat sends recent channel context to OpenRouter and posts the response.
2. `/ask` runs the same chat pipeline with configurable history and memory opt-out.
3. `/memory` lets users inspect and remove their own memory. Admins can target other users or server memory.
4. `/imagine` generates images through OpenRouter, with an optional reference image.
5. Reminders and events are created from natural language, stored as JSON, and fired by a scheduler.

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
- Use `{bot}` in a description when it should include `BOT_NAME`. `resolveDescription` fills it in at registration time.

Handler rules:

- MUST use `s.InteractionRespond` to respond.
- MUST respond exactly once. If a response fails, log it.
- MUST use user-friendly messages and say "server", not "guild".
- MUST log failures before responding.

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
- Images from the current message and the replied-to message are attached as image content parts. Older context images are not sent.
- Context messages include UTC RFC3339 timestamps.
- Earlier messages are marked as reference only. The final user message is the request, and older context may be unrelated.
- A system prompt tells the bot its stack: Go, discordgo, Railway, OpenRouter, and DeepSeek V4.1 Flash 1017.
- It also tells the bot that memory is automatic, tool-free, does not require a mention, and covers facts people share about themselves plus admin-taught server facts. It must not claim it cannot remember between sessions, and it must not claim a write it cannot store.
- It requires inline Discord markdown citations for web search results, using short relevant phrases as link text and at most two citations by default.
- It requires Discord mention and timestamp formatting: `<@id>`, `<#id>`, and `<t:unix:R>`.
- It makes replies peer-like: casual, direct, and free of unsolicited remediation.
- It bans em dashes and asks for short, simple sentences.
- It also limits replies to a single tweet (280 characters) unless the user asks for more detail.
- The default prompt is a template. `{{bot}}` becomes `BOT_NAME`, `{{community}}` becomes `COMMUNITY_NAME`, and `{{model}}` becomes `OPENROUTER_MODEL`.
- The system prompt can be overridden with `CHAT_SYSTEM_PROMPT`.
- Do not hardcode The Coffee Break in new prompts or user-facing text.

Response rules:

- Reply to the triggering message.
- Allow user mentions with `AllowedMentions`, but block everyone and role mentions. Keep `RepliedUser: false`.
- Append the current Unix time to the system prompt so Discord timestamp output is accurate.
- Split responses over Discord's 2000-character limit, up to `CHAT_MAX_CHUNKS`.
- Use a typing indicator while waiting.
- Rate limit per user and cap concurrent requests.

---

## Vision

Location: `internal/server/vision.go`

- Attach images from the current message and the replied-to message.
- Do not attach images from older context messages.
- Accept PNG, JPEG, WebP, and GIF only.
- Enforce `VISION_MAX_IMAGES` and `VISION_MAX_IMAGE_MB`.
- Only accept Discord CDN attachment URLs.
- Send a short text label before each image.
- Use either the original URL or the Discord proxy URL.
- Do not use images for passive memory extraction.

The OpenRouter message format uses a content array with a text part first and `image_url` parts after it. Text-only messages must stay JSON strings.

---

## Reminders and Events

Location: `internal/reminders/` and `internal/server/reminders.go`

- Reminders are private records. Events are public and joinable.
- Natural language is parsed by a dedicated JSON LLM call, not the main chat call.
- Users must set a timezone with `/timezone` before creating reminders.
- Store data in `REMINDERS_PATH` as JSON with atomic writes.
- The scheduler ticks every `REMINDER_TICK_SECONDS`.
- Recurrence supports daily, weekly, monthly, and interval.
- Next run times are computed in the user's timezone and stored in UTC.
- Missed reminders older than `REMINDER_GRACE_MINUTES` are skipped.
- Failed sends retry after `REMINDER_RETRY_MINUTES` up to `REMINDER_MAX_ATTEMPTS`.
- Announcements are public and joinable by replying with `me too`, `+1`, and similar.
- Join and leave replies must be intercepted before normal chat.
- Trigger messages mention the creator and participants.
- `/reminders` is ephemeral and supports list, cancel, and leave.
- Do not use the main chat memory or web search for reminder parsing or writing.

---

## Image Generation

Location: `internal/imagegen/` and `internal/server/commands/imagine.go`

- Uses the OpenRouter Image API at `POST /api/v1/images`.
- Default model is `black-forest-labs/flux-3-image-klein`.
- Supports one optional reference image via `input_references`.
- Reference images are downloaded from Discord, validated, and encoded as base64 data URLs.
- Requests `b64_json` and uploads the generated bytes as Discord attachments.
- If attaching to the deferred response fails, fall back to a follow-up message with the files.
- Rate limit image generation per user separately from chat.
- Never log full prompts or image bytes.
- Black Forest Labs `safety_tolerance` is passed through `provider.options`.
- `IMAGE_SAFETY_TOLERANCE` sets the default, default `2`. Users can pass `0-4` per request in any channel.
- Flux 3 Image allows `0-4`. Sending above the model max returns 422.
- Higher tolerance does not allow illegal content.
- Detect `imagegen.ContentBlockedError` and show the provider reason.
- Detect `imagegen.ProviderError` and show the provider reason.
- Detect `imagegen.RateLimitError`. Read `Retry-After` first, then a duration in the provider message, then guess 30 seconds.
- The default limit is 20 images per user per hour. Users start with a full bucket, then refill one image every 3 minutes.
- Show local and provider rate limits with a Discord relative timestamp, for example `<t:1234567890:R>`.
- `ratelimit.Limiter.RetryAfter` reports when the next token is available. Use it for the per-user limit.
- Append the failed prompt with `withImagePrompt` so users can copy the `/imagine prompt:...` command.
- Keep unrelated failures on the generic error message.

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
- Also extract clear facts the speaker states about the server, using subject ID `server`.
- Server writes require an Administrator or guild owner and an explicit remember instruction in the message.
- Gate server writes with `isGuildAdmin` and fail closed when permissions cannot be resolved.
- Ignore statements about other people, even when the speaker mentions them.
- Ignore bot messages, other bots, and other users' messages.
- Never write facts about another user based on what someone else said.
- Skip channels listed in `MEMORY_IGNORE_CHANNELS`.
- Filter out secrets, credentials, addresses, health data, and sensitive personal data.
- Filter by `MEMORY_MIN_CONFIDENCE`.
- Deduplicate against existing markdown.
- Write atomically with a temporary file and rename.
- Use JSON response format and enough output tokens to avoid truncated JSON.
- Server memory is admin-managed: explicit admin remember instructions plus manual edits.
- When passive memory is enabled, mention and reply chat skips its own extraction to avoid duplicate calls. `/ask` still extracts because it has no message event.

Rules:

- Memory is enabled for normal messages when `MEMORY_PASSIVE_ENABLED=true`.
- `disable_memory` on `/ask` skips reads and writes.
- The system prompt must say memory is automatic, covers self facts and admin-taught server facts, and needs no memory tool.
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
- `BOT_NAME` and `COMMUNITY_NAME` control branding. Defaults are `Caffeinator` and `The Coffee Break`. `internal/config/branding.go` provides the helpers.
- The Discord username and avatar are set in the Discord Developer Portal, not by env vars.
- `OPENROUTER_APP_URL` sets the referer sent to OpenRouter for attribution.
- `OPENROUTER_API_KEY` is environment-only and should never be added as a flag.
- `.env` is gitignored. Keep `.env.example` current.
- `OPENROUTER_API_KEY` is required. The bot exits at startup when it is missing.
- `CHAT_SYSTEM_PROMPT` overrides the built-in peer-tone prompt.
- Web search is enabled by default with `OPENROUTER_WEB_SEARCH=true`. It is capped by `OPENROUTER_WEB_SEARCH_MAX_RESULTS` and `OPENROUTER_WEB_SEARCH_MAX_USES` to control cost.
- Memory is enabled by default with `MEMORY_ENABLED=true`.
- `MEMORY_DIR` defaults to `./memory`. On Railway, use a volume and set `MEMORY_DIR=/data/memory`.
- `MEMORY_MAX_CONTEXT_CHARS`, `MEMORY_MIN_CONFIDENCE`, and `MEMORY_EXTRACT_TIMEOUT_SECONDS` control memory size and extraction.
- `MEMORY_PASSIVE_ENABLED`, `MEMORY_BATCH_SECONDS`, `MEMORY_MAX_BATCH_MESSAGES`, and `MEMORY_IGNORE_CHANNELS` control passive learning.
- `IMAGE_ENABLED`, `IMAGE_MODEL`, `IMAGE_DEFAULT_RESOLUTION`, `IMAGE_DEFAULT_ASPECT_RATIO`, `IMAGE_SAFETY_TOLERANCE`, `IMAGE_MAX_REFERENCE_MB`, `IMAGE_TIMEOUT_SECONDS`, and `IMAGE_RATE_LIMIT_PER_HOUR` control image generation.
- `REMINDERS_ENABLED`, `REMINDERS_PATH`, `REMINDER_TICK_SECONDS`, `REMINDER_GRACE_MINUTES`, `REMINDER_MAX_PER_USER`, `REMINDER_RETRY_MINUTES`, and `REMINDER_MAX_ATTEMPTS` control reminders.
- `VISION_ENABLED`, `VISION_MAX_IMAGES`, `VISION_MAX_IMAGE_MB`, and `VISION_DETAIL` control image understanding.

---

## Testing

- Prefer pure functions for logic that can be tested without Discord.
- Use interfaces for Discord and LLM dependencies in `chat.go`.
- Use `httptest` for the OpenRouter chat and image clients.
- Do not call live Discord or OpenRouter APIs in tests.
- Test the memory store with `t.TempDir()`.
- Test extraction with a fake chat client.
- Test reminder recurrence with fixed times and IANA timezones.
- Test the reminder store and scheduler with `t.TempDir()` and a fake trigger.
- Run `go test ./...`, `go vet ./...`, and `go build ./...` before finishing.

---

## Go Style

- Keep packages small and focused.
- Wrap errors with context using `%w`.
- Use `context.Context` for network calls.
- Avoid hidden global state except the command registry and rate limiter.
- Use the built-in `min` instead of a custom helper.
- Prefer explicit code over clever abstractions.
