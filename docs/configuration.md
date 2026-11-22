# Configuration

Configuration uses Viper and dotenv. It is environment-only: there are no command-line flags. Environment variables override defaults. Set values in `.env` or in your hosting platform.

## Branding

| Variable | Default | Effect |
|---|---|---|
| `BOT_NAME` | `Caffeinator` | Name the bot uses when it refers to itself. Used in the default system prompt, the `/ask` description, and OpenRouter attribution. |
| `COMMUNITY_NAME` | `The Coffee Break` | Community name used in the default system prompt. |
| `OPENROUTER_APP_URL` | `https://github.com/metruzanca/caffeinator` | Referer sent to OpenRouter for attribution. Point it at your fork. |

The Discord username, avatar, and description are set in the [Discord Developer Portal](https://discord.com/developers/applications). `BOT_NAME` only changes how the bot talks about itself.

## System prompt

`CHAT_SYSTEM_PROMPT` replaces the built-in prompt entirely. The default prompt:

- Tells the bot it is `BOT_NAME`, a Discord bot for `COMMUNITY_NAME`.
- Describes its chat, search, vision, image, reminder, memory, and meme features.
- Makes replies peer-like, casual, and direct.
- Bans em dashes and asks for short, simple sentences.
- Limits replies to 280 characters unless the user asks for more.

## Models

- `OPENROUTER_MODEL` sets the chat model.
- `IMAGE_MODEL` sets the image model.
- `OPENROUTER_BASE_URL` overrides the API base URL for testing.

## Feature toggles

`CHAT_ENABLED`, `VISION_ENABLED`, `IMAGE_ENABLED`, `REMINDERS_ENABLED`, `MEMORY_ENABLED`, and `MEMES_ENABLED` all default to `true`.

`MEMORY_PASSIVE_ENABLED` controls automatic learning from normal messages.

`OPENROUTER_WEB_SEARCH` controls web search.

Meme memory needs `REDDIT_SESSION`. Without it the bot logs a warning and the
scheduled scrape is skipped. See [meme memory](memes.md).

## Environment reference

| Variable | Default | Description |
|---|---|---|
| `TOKEN` | required | Discord bot token |
| `BOT_NAME` | `Caffeinator` | Name the bot uses when it refers to itself |
| `COMMUNITY_NAME` | `The Coffee Break` | Community name used in the default system prompt |
| `CHANNEL_ID` | empty | Optional channel for startup notifications |
| `STARTUP_MESSAGE` | `true` | Print startup info and send the restart notice |
| `LOG_FORMAT` | `text` | `text` or `json`. Automatically `json` when `RAILWAY_ENVIRONMENT` is set |
| `OPENROUTER_API_KEY` | required | OpenRouter API key |
| `OPENROUTER_MODEL` | `deepseek/deepseek-v4.1-flash-1017` | OpenRouter model slug |
| `OPENROUTER_BASE_URL` | `https://openrouter.ai/api/v1` | Override for testing |
| `OPENROUTER_APP_URL` | `https://github.com/metruzanca/caffeinator` | OpenRouter attribution URL |
| `OPENROUTER_WEB_SEARCH` | `true` | Let the model search the web when needed |
| `OPENROUTER_WEB_SEARCH_MAX_RESULTS` | `3` | Maximum results per search |
| `OPENROUTER_WEB_SEARCH_MAX_USES` | `2` | Maximum searches per chat request |
| `CHAT_ENABLED` | `true` | Enable mention chat |
| `CHAT_SYSTEM_PROMPT` | built-in prompt | Override the default system prompt |
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
| `IMAGE_MODEL` | `black-forest-labs/flux-3-image-klein` | OpenRouter image model |
| `IMAGE_DEFAULT_RESOLUTION` | `1K` | Default image resolution |
| `IMAGE_DEFAULT_ASPECT_RATIO` | `1:1` | Default image aspect ratio |
| `IMAGE_SAFETY_TOLERANCE` | `2` | Default moderation tolerance |
| `IMAGE_MAX_REFERENCE_MB` | `8` | Maximum reference image size |
| `IMAGE_TIMEOUT_SECONDS` | `120` | Image generation timeout |
| `IMAGE_RATE_LIMIT_PER_HOUR` | `20` | Image generations per user per hour |
| `REMINDERS_ENABLED` | `true` | Enable reminders and events |
| `REMINDERS_PATH` | `./reminders.json` | Reminder JSON file. Use `/data/reminders.json` in production |
| `REMINDER_TICK_SECONDS` | `15` | Scheduler tick interval |
| `REMINDER_GRACE_MINUTES` | `60` | How late a missed reminder may still fire |
| `REMINDER_MAX_PER_USER` | `50` | Maximum active reminders per user |
| `REMINDER_RETRY_MINUTES` | `2` | Delay before retrying a failed reminder |
| `REMINDER_MAX_ATTEMPTS` | `5` | Maximum send attempts before giving up |
| `MEMORY_ENABLED` | `true` | Enable markdown memory |
| `MEMORY_DIR` | `./memory` | Memory directory. Use `/data/memory` with a volume |
| `MEMORY_MAX_CONTEXT_CHARS` | `3000` | Maximum memory characters injected into a chat request |
| `MEMORY_MIN_CONFIDENCE` | `0.7` | Minimum confidence for a stored fact |
| `MEMORY_EXTRACT_TIMEOUT_SECONDS` | `30` | Background memory extraction timeout |
| `MEMORY_PASSIVE_ENABLED` | `true` | Learn from normal server messages automatically |
| `MEMORY_BATCH_SECONDS` | `60` | Batch window for passive learning |
| `MEMORY_MAX_BATCH_MESSAGES` | `20` | Maximum messages per user batch |
| `MEMORY_IGNORE_CHANNELS` | empty | Comma-separated channel IDs to skip for passive learning |
| `VISION_ENABLED` | `true` | Enable image understanding for attachments |
| `VISION_MAX_IMAGES` | `4` | Maximum images per chat request |
| `VISION_MAX_IMAGE_MB` | `8` | Maximum image size in MB |
| `VISION_DETAIL` | `auto` | Image detail: `auto`, `low`, `high`, or `original` |
| `MEMES_ENABLED` | `true` | Enable meme memory |
| `MEMES_CRON` | `0 15 * * 1,3,5` | Meme scrape cron in `MEMES_TIMEZONE` |
| `MEMES_TIMEZONE` | `UTC` | Timezone for the meme cron |
| `MEMES_SUBREDDITS` | `ProgrammerHumor` | Comma-separated subreddits |
| `MEMES_TOP_PERIOD` | `week` | Top listing period |
| `MEMES_SCAN_LIMIT` | `25` | Posts read per subreddit |
| `MEMES_POSTS_PER_RUN` | `5` | New memes analyzed per run |
| `MEMES_MAX_STORED` | `50` | Maximum stored memes |
| `MEMES_RETENTION_DAYS` | `30` | Age limit for unused, unreacted memes |
| `MEMES_RELEVANCE_LIMIT` | `3` | Memes injected into a chat request |
| `MEMES_MAX_IMAGE_MB` | `8` | Maximum meme image size |
| `MEMES_ANALYSIS_MODEL` | empty | Vision model. Defaults to `OPENROUTER_MODEL` |
| `MEMES_ANALYSIS_TIMEOUT_SECONDS` | `60` | Vision call timeout per meme |
| `MEMES_REFRESH_TIMEOUT_SECONDS` | `600` | Whole scrape timeout |
| `MEMES_PATH` | `MEMORY_DIR/memes.json` | Meme memory file |
| `MEMES_DIR` | `MEMORY_DIR/memes` | Meme image directory |
| `MEMES_INCLUDE_NSFW` | `false` | Allow NSFW top posts |
| `MEMES_POSITIVE_REACTIONS` | see [meme memory](memes.md) | Reactions that mark a meme as good |
| `REDDIT_SESSION` | empty | Reddit session cookie. Required for meme memory |
| `REDDIT_USER_AGENT` | Firefox UA | Browser identity sent to Reddit |

`TOKEN` and `OPENROUTER_API_KEY` are required. The bot exits at startup when either is missing.

## Railway notes

- Use the repository root as the build directory.
- Mount a volume at `/data`.
- Set `MEMORY_DIR=/data/memory` and `REMINDERS_PATH=/data/reminders.json`.
- Meme memory defaults to `/data/memory/memes.json` and `/data/memory/memes` when `MEMORY_DIR` is set. Set `REDDIT_SESSION` to enable scraping.
- Railway provides `RAILWAY_GIT_COMMIT_SHA` and `RAILWAY_GIT_COMMIT_MESSAGE`, which the restart notice uses.

## Restart notice

When `CHANNEL_ID` is set and `STARTUP_MESSAGE` is enabled, the bot posts the commit it was built from:

```text
Bot has restarted. Built from `74d7bc3` feat(chat): require compact inline citations for web search
```

Local builds fall back to the git revision Go embeds at build time, so the commit message can be missing there. The notice never pings anyone.
