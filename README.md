<div align="center">
  <img src="checkpoint-bot/.github/Remote_Caffeinator.png" alt="Remote Caffeinator" width="200">

  # ☕ The Coffee Break

  **Infrastructure for "The Coffee Break" Discord community**

</div>

---

## Overview

This monorepo contains the tooling for "The Coffee Break" Discord server, an accountability group for developers.

## Quick Start

### Prerequisites

- [mise](https://mise.jdx.dev/) for tooling management
- Discord bot token from [Discord Developer Portal](https://discord.com/developers/applications)

### Setup

A lightweight Discord bot for The Coffee Break. Fixes X/Twitter and Instagram links for better embeds, chats when mentioned through OpenRouter, and slowly learns durable facts from conversations it is invoked in.

```bash
cd checkpoint-bot
go mod download
go build -o checkpoint-bot .
./checkpoint-bot --TOKEN="your-bot-token"
```

See [checkpoint-bot/readme.md](./checkpoint-bot/readme.md) for full documentation.

---

## License

[MPL-2.0](./checkpoint-bot/LICENSE)
