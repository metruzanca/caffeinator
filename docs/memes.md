# Meme memory

Meme memory keeps Caffeinator up to date with programming memes. It scrapes a
subreddit top listing on a schedule, understands each image, and shares a
relevant meme when one fits the conversation. Memes are never posted on a
schedule.

## Setup

Reddit blocks anonymous listing requests from most servers. A signed-in
session cookie is required.

1. Sign in at `https://old.reddit.com`.
2. Open DevTools, then Application, then Cookies, then `https://old.reddit.com`.
3. Copy the `reddit_session` value.
4. Set `REDDIT_SESSION` to that value.

Both the bare value and a full `reddit_session=...` cookie header are accepted.
Treat the cookie like a password. The bot never logs it.

## Schedule

`MEMES_CRON` uses a standard 5-field cron expression in `MEMES_TIMEZONE`:

```text
MEMES_CRON=0 15 * * 1,3,5
MEMES_TIMEZONE=UTC
```

That runs Monday, Wednesday, and Friday at 15:00 UTC. Use `/memes refresh` to
run immediately.

The scraper reads `MEMES_TOP_PERIOD` from the subreddit top listing. The default
is `week`, matching `https://reddit.com/r/ProgrammerHumor/top/?t=week`.

Requests are serialized with a 2 second gap. Each run reads up to
`MEMES_SCAN_LIMIT` posts and analyzes at most `MEMES_POSTS_PER_RUN` new memes.

## What is stored

Each meme is saved in `memory/memes.json` with:

- Reddit post id, subreddit, title, score, and permalink.
- A one-line caption.
- A short explanation of the joke or reference.
- Search tags and `use_when` phrases.
- The local image file.
- Use count, reaction count, and the Discord messages that carried it.

Images live in `memory/memes/`. Both paths derive from `MEMORY_DIR` unless you
set `MEMES_PATH` or `MEMES_DIR` directly.

Only image posts are stored. Text posts, link posts, and videos are skipped.
For a gallery, the cover image is enough for the model to understand the joke.

## Using memes in chat

On every chat request with memory enabled, the bot scores stored memes against
the prompt and the last two messages. Scoring uses tags, `use_when` phrases,
caption words, recency, and reactions. The top `MEMES_RELEVANCE_LIMIT` matches
are added to the system prompt.

The model is told to use one meme only when it directly fits. When it does, it
emits a marker such as `[meme:abc123]`. The bot removes the marker and attaches
the stored image to its reply.

Memes are not injected when a request uses `/ask disable_memory:true`.
`MEMES_ENABLED=false` turns the whole meme feature off.

## Reactions

The bot watches reactions only on its own reply messages that carried a meme.
Positive reactions make the meme more likely to be used again.

The default allowlist is:

```text
😂 🤣 💀 🔥 👍 ❤️ 😆
```

Change it with `MEMES_POSITIVE_REACTIONS`. Negative reactions are ignored. The
bot ignores its own reactions and reactions from other bots. Each human reactor
counts once per meme.

## Retirement

Memes do not stay forever:

- Unused and unreacted memes older than `MEMES_RETENTION_DAYS` are deleted.
- `MEMES_MAX_STORED` caps the total.
- When the cap is reached, memes with no reactions are evicted first, then the
  least used, then the oldest.
- Reacted or used memes survive age pruning.
- Image files for retired memes are deleted, and orphan images are cleaned up.

A retired meme can appear again later if it is still a top post.

## Commands

```text
/memes status
/memes refresh
/memes list [count]
```

All three are admin only. `status` shows the schedule, next run, last result,
storage count, and reaction total. `refresh` runs a scrape now. `list` shows
recent memes with their tags, caption, and link.

## Configuration

| Variable | Default | Effect |
|---|---|---|
| `MEMES_ENABLED` | `true` | Enable meme memory. |
| `MEMES_CRON` | `0 15 * * 1,3,5` | Scrape schedule. |
| `MEMES_TIMEZONE` | `UTC` | Timezone for the cron expression. |
| `MEMES_SUBREDDITS` | `ProgrammerHumor` | Comma-separated subreddits. |
| `MEMES_TOP_PERIOD` | `week` | `hour`, `day`, `week`, `month`, `year`, or `all`. |
| `MEMES_SCAN_LIMIT` | `25` | Posts read per subreddit. |
| `MEMES_POSTS_PER_RUN` | `5` | New memes analyzed per run. |
| `MEMES_MAX_STORED` | `50` | Maximum stored memes. |
| `MEMES_RETENTION_DAYS` | `30` | Age limit for unused, unreacted memes. |
| `MEMES_RELEVANCE_LIMIT` | `3` | Memes injected into a chat request. |
| `MEMES_MAX_IMAGE_MB` | `8` | Maximum image size. |
| `MEMES_ANALYSIS_MODEL` | empty | Vision model. Defaults to `OPENROUTER_MODEL`. |
| `MEMES_ANALYSIS_TIMEOUT_SECONDS` | `60` | Vision call timeout per meme. |
| `MEMES_REFRESH_TIMEOUT_SECONDS` | `600` | Whole scrape timeout. |
| `MEMES_PATH` | `MEMORY_DIR/memes.json` | Meme memory file. |
| `MEMES_DIR` | `MEMORY_DIR/memes` | Meme image directory. |
| `MEMES_INCLUDE_NSFW` | `false` | Allow NSFW top posts. |
| `MEMES_POSITIVE_REACTIONS` | see above | Reactions that mark a meme as good. |
| `REDDIT_SESSION` | empty | Required Reddit session cookie. |
| `REDDIT_USER_AGENT` | Firefox UA | Browser identity sent to Reddit. |

## Limitations

- Reddit can change its listing markup. Parsing failures are logged and shown in
  `/memes status`.
- Only the first gallery image is stored.
- The model analyzes a still image. Animated GIFs may lose timing or motion.
- Reddit rate limits and login walls can skip a run. The next scheduled run
  retries.
