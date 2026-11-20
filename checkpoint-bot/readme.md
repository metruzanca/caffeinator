<div align="center">
  <img src=".github/Remote_Caffeinator.png" alt="Risk of Rain 2's remote caffeinator" width="200">

  # ☕ Caffeinator

  **A small Discord bot for The Coffee Break**

  [![Go Version](https://img.shields.io/badge/go-1.25-blue.svg)](https://golang.org)
  [![License](https://img.shields.io/badge/license-MPL%202.0-brightgreen.svg)](LICENSE)

</div>

---

## What it does

Caffeinator is a lightweight, stateless Discord bot with two jobs:

- **Fix social links** so Discord renders proper embeds.
- **Chat when mentioned** using an LLM through OpenRouter, with optional live web search.

Checkpoint and goal tracking were removed. If that feature comes back, it will start from a fresh design.

---

## Commands

| Command | Description |
|---|---|
| `/link url:<url>` | Rewrites X/Twitter links to `fxtwitter.com` and Instagram links to `kkinstagram.com`, and removes tracking query parameters. |

Examples:

```text
/link url:https://x.com/4ster_light/status/2116158570198622382?s=20
→ https://fxtwitter.com/4ster_light/status/2116158570198622382

/link url:https://www.instagram.com/p/ABC123/?igsh=tracking
→ https://kkinstagram.com/p/ABC123
```

The rewritten link is posted publicly so everyone can click or preview it.

---

## Mention chat

Mention the bot, or reply to one of its messages, and it will answer through OpenRouter.

- **Mention:** `@Caffeinator what do you think about this?`
- **Reply:** reply directly to a bot message, with or without an `@`.

Context behavior:

- A plain mention includes the last 10 messages from the channel.
- A reply uses the replied-to message plus up to 9 messages before it.
- If the replied-to message is gone or inaccessible, it falls back to the last 10 messages before the mention.
- Bot messages are sent as `assistant` turns. Everything else is a `user` turn with the author name prefixed.
- A short system prompt keeps replies to a single tweet, 280 characters or fewer, unless the user explicitly asks for more detail. Override it with `CHAT_SYSTEM_PROMPT`.

Web search:

- The model can search the web when it decides it needs current information.
- Searches are capped at 2 per request and 3 results per search by default.
- Citations come back as normal markdown links.
- Web search uses OpenRouter credits, roughly $0.007 per search with the default Exa engine, plus model token costs. Casual mentions that do not need current information should not search at all.

Other behavior:

- Other bots and the bot's own messages are ignored.
- Replies and mentions are rate limited per user.
- Responses are capped at 3 Discord messages.
- The model cannot ping `@everyone`, roles, or users in its reply.
- Guild channels only for now.

Messages in the context window are sent to OpenRouter. Do not mention the bot in channels where that is not acceptable.

---

## Setup

### Prerequisites

- Go 1.25+ or Docker
- A Discord application and bot token from the [Discord Developer Portal](https://discord.com/developers/applications)
- An [OpenRouter](https://openrouter.ai/) API key for mention chat

### Required Discord settings

1. Enable **Message Content Intent** under **Bot → Privileged Gateway Intents**.
   Without it, the bot cannot read message text and will not respond to mentions.
2. The bot needs these permissions:
   - View Channels
   - Send Messages
   - Send Messages in Threads
   - Read Message History

The generated invite link requests exactly these permissions and includes the `bot` and `applications.commands` scopes.

### Environment

Copy `.env.example` to `.env` and fill it in, or set the same variables in your host:

| Variable | Default | Description |
|---|---|---|
| `TOKEN` | required | Discord bot token |
| `CLIENT_ID` | empty | Discord application client ID, used by `checkpoint invite` |
| `CHANNEL_ID` | empty | Optional channel for startup notifications |
| `STARTUP_MESSAGE` | `true` | Print startup info and send the restart notice |
| `LOG_FORMAT` | `text` | `text` or `json`. Automatically `json` when `RAILWAY_ENVIRONMENT` is set |
| `OPENROUTER_API_KEY` | empty | Enables mention chat when set |
| `OPENROUTER_MODEL` | `deepseek/deepseek-v4.1-flash-1017` | OpenRouter model slug |
| `OPENROUTER_BASE_URL` | `https://openrouter.ai/api/v1` | Override for testing |
| `OPENROUTER_WEB_SEARCH` | `true` | Let the model search the web when needed |
| `OPENROUTER_WEB_SEARCH_MAX_RESULTS` | `3` | Maximum results per search |
| `OPENROUTER_WEB_SEARCH_MAX_USES` | `2` | Maximum searches per chat request |
| `CHAT_ENABLED` | `true` | Set to `false` to disable mention chat |
| `CHAT_SYSTEM_PROMPT` | built-in 280-character limit | Override the default system prompt |
| `CHAT_CONTEXT_MESSAGES` | `10` | Messages to include, including the reply anchor |
| `CHAT_MAX_TOKENS` | `2048` | Maximum output tokens |
| `CHAT_MAX_CONTEXT_CHARS` | `20000` | Maximum context size in characters |
| `CHAT_MAX_CHARS_PER_MESSAGE` | `4000` | Per-message context limit |
| `CHAT_MAX_CHUNKS` | `3` | Maximum Discord messages per response |
| `CHAT_TIMEOUT_SECONDS` | `60` | OpenRouter request timeout |
| `CHAT_MAX_CONCURRENT` | `4` | Concurrent model requests |
| `CHAT_RATE_LIMIT_CAPACITY` | `1` | Requests allowed per user per window |
| `CHAT_RATE_LIMIT_SECONDS` | `5` | Rate limit window |

If `OPENROUTER_API_KEY` is empty, the bot still runs and `/link` works. Mention chat is disabled with a startup warning.

### Run from source

```bash
go mod download
go build -o checkpoint-bot .
./checkpoint-bot
```

### Docker

```bash
docker build -t caffeinator .
docker run -d \
  --name caffeinator \
  -e TOKEN="your-bot-token" \
  -e OPENROUTER_API_KEY="your-openrouter-key" \
  caffeinator
```

There is no database or volume to mount.

### Invite link

```bash
./checkpoint-bot invite
```

or, while the bot is running with `STARTUP_MESSAGE=true`, the invite link is printed on startup.

---

## Development

### Project structure

```text
checkpoint-bot/
├── cmd/                       # CLI commands (run, invite, command clear)
├── internal/
│   ├── config/                # Viper and dotenv configuration
│   ├── llm/                   # OpenRouter client
│   ├── ratelimit/             # Shared token bucket limiter
│   ├── server/                # Bot lifecycle and mention chat
│   │   └── commands/          # Slash commands
│   └── util/                  # Small helpers
└── main.go
```

### Adding a slash command

1. Create a file in `internal/server/commands/`.
2. Define a `Command` with `ApplicationCommand` and a `Handler`.
3. Register it in `init()` with `registerCommand(...)`.
4. Run `go test ./...` and `go vet ./...`.

### Tests

```bash
go test ./...
go vet ./...
```

The suite covers link rewriting, context assembly, mention detection, response chunking, rate limiting, and the OpenRouter client (with `httptest`, no live API calls).

### Logging

Logs go to stdout. Locally they use charmbracelet's text format. On Railway they switch to JSON automatically so Railway can read the `level` field and color info, warn, and error correctly.

Use `github.com/charmbracelet/log` with key-value pairs:

```go
log.Info("chat request", "channel", channelID, "user", userID)
log.Error("request failed", "err", err, "channel", channelID)
```

Do not log message contents or API keys.

---

## License

Mozilla Public License 2.0 (MPL-2.0). See [LICENSE](LICENSE).
