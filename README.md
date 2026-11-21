<div align="center">
  <img src=".github/Remote_Caffeinator.png" alt="Risk of Rain 2's remote caffeinator" width="200">

  # ☕ Caffeinator

  **A self-hostable AI Discord bot with chat, image generation, reminders, and memory**

  [![Go Version](https://img.shields.io/badge/go-1.25-blue.svg)](https://golang.org)
  [![License](https://img.shields.io/badge/license-MPL%202.0-brightgreen.svg)](LICENSE)

</div>

---

## Features

Caffeinator is an AI Discord bot written in Go with [discordgo](https://github.com/bwmarrin/discordgo). It runs as a single process and stores its state as files on disk.

- **LLM chat.** Mention the bot or reply to it. Chat uses [OpenRouter](https://openrouter.ai/) with optional web search and image understanding. [Details](docs/chat.md)
- **Image generation.** `/imagine` generates or edits images through OpenRouter. [Details](docs/image-generation.md)
- **Reminders and events.** Natural language reminders, joinable events, and recurring schedules. [Details](docs/reminders.md)
- **Memory.** Learns durable facts and stores them as editable markdown. [Details](docs/memory.md)

## Commands

| Command | Description |
|---|---|
| `/imagine prompt:<text> [image] [aspect_ratio] [resolution] [safety_tolerance]` | Generates an image, optionally from a reference image. |
| `/ask text:<text> [history_length] [disable_memory]` | Runs the chat pipeline with more context control. |
| `/timezone zone:<iana>` | Sets your timezone for reminders. |
| `/reminders list\|cancel\|leave` | Lists and manages reminders and events. |
| `/memory view\|forget\|list` | Views or forgets memory. Admins can manage server memory. |

---

## Quick start on Railway

Railway is the recommended way to run Caffeinator. It builds the repo and keeps your data on a volume.

1. Create a Discord application in the [Discord Developer Portal](https://discord.com/developers/applications).
2. Under **Bot**, enable **Message Content Intent** and copy the bot token.
3. Create an [OpenRouter](https://openrouter.ai/) API key.
4. Create a Railway project from your fork of this repo.
5. Add a volume mounted at `/data`.
6. Set these variables:

```text
TOKEN=your-discord-bot-token
CLIENT_ID=your-discord-client-id
OPENROUTER_API_KEY=your-openrouter-key
MEMORY_DIR=/data/memory
REMINDERS_PATH=/data/reminders.json
```

7. Deploy. Railway detects Go from `go.mod` at the repository root.

`TOKEN`, `CLIENT_ID`, and `OPENROUTER_API_KEY` are required. The bot exits at startup if the Discord token or OpenRouter key is missing.

The bot prints an invite link on startup when `STARTUP_MESSAGE=true`. You can also run `caffeinator invite` locally.

Required Discord permissions: View Channels, Send Messages, Send Messages in Threads, Read Message History, Attach Files, and Add Reactions. The invite link includes the `bot` and `applications.commands` scopes.

## Customization

Everything works with defaults. Change only what you need.

| Variable | Default | Effect |
|---|---|---|
| `BOT_NAME` | `Caffeinator` | Name the bot uses when it refers to itself. |
| `COMMUNITY_NAME` | `The Coffee Break` | Community name used in the default system prompt. |
| `CHAT_SYSTEM_PROMPT` | built-in prompt | Replaces the prompt entirely. |
| `OPENROUTER_MODEL` | `deepseek/deepseek-v4.1-flash-1017` | Chat model. |
| `IMAGE_MODEL` | `black-forest-labs/flux-3-image-klein` | Image model. |

Feature toggles: `CHAT_ENABLED`, `VISION_ENABLED`, `IMAGE_ENABLED`, `REMINDERS_ENABLED`, and `MEMORY_ENABLED`.

The real Discord username, avatar, and description are set in the Discord Developer Portal. `BOT_NAME` only changes how the bot talks about itself.

See [configuration](docs/configuration.md) for every variable.

---

## Need anything else?

The README stays short on purpose. For self-hosting on another platform, custom tuning, or a setup that does not fit the steps above, copy this prompt into your preferred AI:

```text
I want to self-host Caffeinator, a Go Discord bot. The repository is checked
out at <path-to-repo>.

Read README.md and every file under docs/. Then walk me through:
1. Deploying it on <platform>.
2. Configuring it for my server.
3. Verifying it works.

Ask me for missing values instead of guessing. Use the platform's official
docs when you are unsure. Do not skip the Discord Developer Portal steps.
```

## Development

```bash
go build ./...
go test ./...
go vet ./...
```

See [development](docs/development.md) for the project layout and conventions.

## License

Mozilla Public License 2.0 (MPL-2.0). See [LICENSE](LICENSE).
