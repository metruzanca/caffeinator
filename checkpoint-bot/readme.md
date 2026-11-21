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

Built with Go and discordgo. Deployed on Railway. Chat uses OpenRouter with DeepSeek V4.1 Flash 1017 by default.

Checkpoint and goal tracking were removed. If that feature comes back, it will start from a fresh design.

---

## Commands

| Command | Description |
|---|---|
| `/link url:<url>` | Rewrites X/Twitter links to `fxtwitter.com` and Instagram links to `kkinstagram.com`, and removes tracking query parameters. |
| `/imagine prompt:<text> [image] [aspect_ratio] [resolution]` | Generates an image. Attach a reference image to edit or restyle it. |
| `/ask text:<text> [history_length] [disable_memory]` | Runs the same chat pipeline as a mention. `history_length` defaults to 10 and can go up to 100. Set `disable_memory` to skip memory reads and writes for that ask. Responses are public. |
| `/memory view\|forget\|list` | View or forget memory. Everyone can manage their own memory. Admins can target other users or server memory and list users. Responses are ephemeral. |

Examples:

```text
/link url:https://x.com/4ster_light/status/2116158570198622382?s=20
→ https://fxtwitter.com/4ster_light/status/2116158570198622382

/link url:https://www.instagram.com/p/ABC123/?igsh=tracking
→ https://kkinstagram.com/p/ABC123
```

The rewritten link is posted publicly so everyone can click or preview it.

---

## Image generation

`/imagine` generates images through OpenRouter using `bytedance-seed/seedream-5-0-flash` by default.

```text
/imagine prompt:a red panda astronaut, studio lighting
/imagine prompt:turn this into a watercolor painting image:<attachment>
```

Options:

- `image` is an optional reference image for editing or style transfer.
- `aspect_ratio` supports `1:1`, `16:9`, `9:16`, `4:3`, `3:4`, `3:2`, `2:3`, and `auto`.
- `resolution` supports `1K`, `1.5K`, and `2K`.

Defaults:

- Resolution `1K`
- Aspect ratio `1:1`
- One image per request
- Five generations per user per hour

Reference images are downloaded from Discord, converted to base64, and sent to OpenRouter. PNG, JPEG, and WebP are accepted. The default limit is 8 MB.

Image generation is billed per image by OpenRouter, not by tokens. Seedream 5.0 Flash costs about $0.018 per image at the time of writing.

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
- A system prompt gives the bot its stack and persistent memory, and makes it reply like a peer: casual, direct, and competent. It avoids unsolicited advice, checklists, and step-by-step remediation. It also avoids em dashes, keeps sentences simple, and limits replies to a single tweet (280 characters or fewer) unless the user asks for more. Override it with `CHAT_SYSTEM_PROMPT`.

Web search:

- The model can search the web when it decides it needs current information.
- Searches are capped at 2 per request and 3 results per search by default.
- Citations come back as inline Discord markdown links, with a short relevant phrase as the link text.
- The prompt asks for at most two citations unless the user asks for more, and forbids inventing URLs.
- Web search uses OpenRouter credits, roughly $0.007 per search with the default Exa engine, plus model token costs. Casual mentions that do not need current information should not search at all.

Discord formatting:

- Users are mentioned with `<@1234567890>`.
- Channels are mentioned with `<#1234567890>`.
- Timestamps use Unix seconds, for example `<t:1795222140:R>` or `<t:1795222140:f>`.
- The bot includes the current Unix time in each request so timestamps stay accurate.
- User mentions are allowed and can ping. `@everyone`, `@here`, and role mentions are blocked.

Images:

- Attach an image to a mention or reply and the bot can see it.
- Images on the replied-to message are also included.
- Supported types: PNG, JPEG, WebP, and GIF.
- Default limits: 4 images per request and 8 MB per image.
- Only the current and replied-to message images are sent, not the whole channel history.
- Image understanding is enabled with `VISION_ENABLED=true`.

Other behavior:

- Other bots and the bot's own messages are ignored.
- Replies and mentions are rate limited per user.
- Responses are capped at 3 Discord messages.
- Guild channels only for now.

Messages in the context window are sent to OpenRouter. Do not mention the bot in channels where that is not acceptable.

---

## Ask command

`/ask` uses the same pipeline as a mention, with two extra controls.

- `history_length` defaults to `10`. It can be raised to `100` for more channel context.
- `disable_memory` skips both memory reads and memory writes for that ask.
- Responses are public, so everyone in the channel can see them.

Example:

```text
/ask text:what did we decide about the deploy? history_length:30
```

---

## Memory

The bot slowly learns durable facts from normal messages and from conversations where it is invoked.

It learns from:

- Normal guild messages, automatically.
- Mentions and replies to the bot.
- `/ask`, unless `disable_memory` is set.
- It ignores bots, DMs, and channels listed in `MEMORY_IGNORE_CHANNELS`.

Memory is stored as markdown on disk:

```text
memory/server.md
memory/users/123456789.md
```

Each user file is human-readable and can be edited by hand. Example:

```markdown
# Alice (123456789)

- 2026-11-20 [work]: Building a Go Discord bot.
- 2026-11-20 [preference]: Prefers short answers.
```

How it works:

- Passive messages are batched per author every 60 seconds by default.
- A background model call extracts high-confidence facts from that author's own messages.
- It only stores first-person facts that people state about themselves.
- Claims about other users are ignored, even if the speaker mentions them.
- It does not learn from other users' messages or from the bot's own replies.
- Facts are deduplicated, filtered for sensitive data, and appended to the markdown files.
- Server memory is not written automatically. Admins can view and edit it manually.
- Memory is automatic. The model does not need a memory write tool and does not need to be mentioned.
- `disable_memory` on `/ask` skips both reads and writes for that ask.
- `MEMORY_PASSIVE_ENABLED=false` disables passive learning.

Memory commands:

```text
/memory view
/memory view user:@someone
/memory view server:true
/memory forget fact:<text>
/memory forget all:true
/memory forget user:@someone fact:<text>
/memory list
```

Rules:

- `/memory view` with no arguments shows your own memory.
- `/memory forget fact:<text>` removes entries that contain the text.
- `/memory forget all:true` clears all memory for you.
- Admins can target other users with `user:@someone` or server memory with `server:true`.
- `/memory list` is admin only.
- Every response is ephemeral, so only the invoker sees it.

### Memory storage

Memory needs persistent storage. On Railway, mount a volume at `/data` and set:

```text
MEMORY_DIR=/data/memory
```

Without a volume, memory is written inside the container and lost on the next deploy.

For local development, the default is `./memory`.

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
   - Attach Files

The generated invite link requests exactly these permissions and includes the `bot` and `applications.commands` scopes.

### Environment

Copy `.env.example` to `.env` and fill it in, or set the same variables in your host:

| Variable | Default | Description |
|---|---|---|
| `TOKEN` | required | Discord bot token |
| `CLIENT_ID` | empty | Discord application client ID, used by `checkpoint invite` |
| `CHANNEL_ID` | empty | Optional channel for startup notifications |
| `STARTUP_MESSAGE` | `true` | Print startup info and send the restart notice with the deployed commit |
| `LOG_FORMAT` | `text` | `text` or `json`. Automatically `json` when `RAILWAY_ENVIRONMENT` is set |
| `OPENROUTER_API_KEY` | empty | Enables mention chat when set |
| `OPENROUTER_MODEL` | `deepseek/deepseek-v4.1-flash-1017` | OpenRouter model slug |
| `OPENROUTER_BASE_URL` | `https://openrouter.ai/api/v1` | Override for testing |
| `OPENROUTER_WEB_SEARCH` | `true` | Let the model search the web when needed |
| `OPENROUTER_WEB_SEARCH_MAX_RESULTS` | `3` | Maximum results per search |
| `OPENROUTER_WEB_SEARCH_MAX_USES` | `2` | Maximum searches per chat request |
| `CHAT_ENABLED` | `true` | Set to `false` to disable mention chat |
| `CHAT_SYSTEM_PROMPT` | built-in peer-tone prompt | Override the default system prompt |
| `CHAT_CONTEXT_MESSAGES` | `10` | Messages to include, including the reply anchor |
| `CHAT_MAX_TOKENS` | `2048` | Maximum output tokens |
| `CHAT_MAX_CONTEXT_CHARS` | `20000` | Maximum context size in characters |
| `CHAT_MAX_CHARS_PER_MESSAGE` | `4000` | Per-message context limit |
| `CHAT_MAX_CHUNKS` | `3` | Maximum Discord messages per response |
| `CHAT_TIMEOUT_SECONDS` | `60` | OpenRouter request timeout |
| `CHAT_MAX_CONCURRENT` | `4` | Concurrent model requests |
| `CHAT_RATE_LIMIT_CAPACITY` | `1` | Requests allowed per user per window |
| `CHAT_RATE_LIMIT_SECONDS` | `5` | Rate limit window |
| `IMAGE_ENABLED` | `true` | Enable `/imagine` |
| `IMAGE_MODEL` | `bytedance-seed/seedream-5-0-flash` | OpenRouter image model |
| `IMAGE_DEFAULT_RESOLUTION` | `1K` | Default image resolution |
| `IMAGE_DEFAULT_ASPECT_RATIO` | `1:1` | Default image aspect ratio |
| `IMAGE_MAX_REFERENCE_MB` | `8` | Maximum reference image size |
| `IMAGE_TIMEOUT_SECONDS` | `120` | Image generation timeout |
| `IMAGE_RATE_LIMIT_PER_HOUR` | `5` | Image generations per user per hour |
| `MEMORY_ENABLED` | `true` | Enable markdown memory |
| `MEMORY_DIR` | `./memory` | Memory directory. Use `/data/memory` with a Railway volume |
| `MEMORY_MAX_CONTEXT_CHARS` | `3000` | Maximum memory characters injected into a chat request |
| `MEMORY_MIN_CONFIDENCE` | `0.7` | Minimum confidence for a stored fact |
| `MEMORY_EXTRACT_TIMEOUT_SECONDS` | `30` | Background memory extraction timeout |
| `MEMORY_PASSIVE_ENABLED` | `true` | Learn from normal guild messages automatically |
| `MEMORY_BATCH_SECONDS` | `60` | Batch window for passive learning |
| `MEMORY_MAX_BATCH_MESSAGES` | `20` | Maximum messages per user batch |
| `MEMORY_IGNORE_CHANNELS` | empty | Comma-separated channel IDs to skip for passive learning |
| `VISION_ENABLED` | `true` | Enable image understanding for attachments |
| `VISION_MAX_IMAGES` | `4` | Maximum images per chat request |
| `VISION_MAX_IMAGE_MB` | `8` | Maximum image size in MB |
| `VISION_DETAIL` | `auto` | Image detail: `auto`, `low`, `high`, or `original` |

If `OPENROUTER_API_KEY` is empty, the bot still runs and `/link` works. Mention chat is disabled with a startup warning.

### Restart notice

When `CHANNEL_ID` is set and `STARTUP_MESSAGE` is enabled, the bot posts the commit it was built from:

```text
Bot has restarted. Built from `74d7bc3` feat(chat): require compact inline citations for web search
```

Railway provides `RAILWAY_GIT_COMMIT_SHA` and `RAILWAY_GIT_COMMIT_MESSAGE` for GitHub deploys. Local builds fall back to the git revision Go embeds at build time, so the commit message can be missing there. The notice never pings anyone.

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
│   ├── imagegen/              # OpenRouter image generation client
│   ├── llm/                   # OpenRouter chat client
│   ├── memory/                # Markdown memory store and extraction
│   ├── ratelimit/             # Shared token bucket limiter
│   ├── server/                # Bot lifecycle and chat pipeline
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

The suite covers link rewriting, context assembly, mention detection, `/ask`, memory storage and extraction, image generation request/response handling, response chunking, rate limiting, and the OpenRouter clients (with `httptest`, no live API calls).

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
