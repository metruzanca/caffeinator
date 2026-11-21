# Memory

Memory is still early. It misses things and the design will keep changing.

The bot slowly learns durable facts from normal messages and from conversations where it is invoked. Facts are stored as human-readable markdown on disk.

## What it learns

- Normal server messages, automatically.
- Mentions and replies to the bot.
- `/ask`, unless `disable_memory` is set.
- Only first-person facts that people state about themselves.
- Server facts taught by an admin with an explicit instruction, for example `@Caffeinator remember that this server is for developers`.

What it ignores:

- Bots, DMs, and channels listed in `MEMORY_IGNORE_CHANNELS`.
- Claims about other users, even when the speaker mentions them.
- Other users' messages when it is learning about you.
- Secrets, credentials, addresses, health data, and other sensitive personal data.

## How it works

- Passive messages are batched per author every `MEMORY_BATCH_SECONDS`, 60 seconds by default.
- A background model call extracts high-confidence facts from that author's own messages.
- Facts are deduplicated and appended to the markdown files.
- Server memory writes need an Administrator or owner account.
- Server memory is shared and loaded into every chat request.
- Memory is automatic. The model does not need a memory tool and does not need to be mentioned.
- `MEMORY_PASSIVE_ENABLED=false` disables passive learning.
- `MEMORY_ENABLED=false` disables the whole feature.

## Storage

```text
memory/server.md
memory/users/123456789.md
```

Each user file can be edited by hand. Example:

```markdown
# Alice (123456789)

- 2026-11-20 [work]: Building a Go Discord bot.
- 2026-11-20 [preference]: Prefers short answers.
```

Set `MEMORY_DIR` to a persistent path in production, for example `/data/memory`. Without persistent storage, memory is lost on deploy.

## Commands

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

## Tuning

- `MEMORY_MAX_CONTEXT_CHARS` caps how much memory is injected into a chat request.
- `MEMORY_MIN_CONFIDENCE` filters low-confidence facts.
- `MEMORY_MAX_BATCH_MESSAGES` caps each passive batch.
- `MEMORY_EXTRACT_TIMEOUT_SECONDS` caps the background extraction call.
