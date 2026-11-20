<div align="center">
  <img src="checkpoint-bot/.github/Remote_Caffeinator.png" alt="Remote Caffeinator" width="200">

  # ☕ The Coffee Break

  **Infrastructure for "The Coffee Break" Discord community**

</div>

---

## Overview

This monorepo contains the tooling and infrastructure for "The Coffee Break;" Discord server—an accountability group for developers.

## Projects

| Directory | Description |
|-----------|-------------|
| [`checkpoint-bot/`](./checkpoint-bot/) | Discord bot for link fixing and mention chat (Go) |
| [`server-config/`](./server-config/) | Terraform configuration for Discord server structure |

## Quick Start

### Prerequisites

- [mise](https://mise.jdx.dev/) for tooling management
- Discord bot token from [Discord Developer Portal](https://discord.com/developers/applications)

### Setup

```bash
# Install tools
mise install

# See individual project READMEs for specific setup
```

## Projects

### Caffeinator (Checkpoint Bot)

A lightweight Discord bot for The Coffee Break. Fixes X/Twitter and Instagram links for better embeds, and chats when mentioned through OpenRouter.

```bash
cd checkpoint-bot
go mod download
go build -o checkpoint-bot .
./checkpoint-bot --TOKEN="your-bot-token"
```

See [checkpoint-bot/readme.md](./checkpoint-bot/readme.md) for full documentation.

### Server Config

Terraform configuration for managing the Discord server structure (channels, roles, categories).

```bash
cd server-config
terraform init
terraform plan
```

Uses Railway S3-compatible storage for state backend.

---

## License

[MPL-2.0](./checkpoint-bot/LICENSE)
