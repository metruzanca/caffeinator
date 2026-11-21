# Chat

Mention the bot, or reply to one of its messages, and it answers through OpenRouter.

- Mention: `@Caffeinator what do you think about this?`
- Reply: reply directly to a bot message, with or without an `@`.

Chat needs `OPENROUTER_API_KEY`. The bot refuses to start without it.

## Context

- A plain mention includes the last `CHAT_CONTEXT_MESSAGES` messages, 10 by default.
- A reply uses the replied-to message plus up to 9 messages before it.
- If the replied-to message is gone or inaccessible, it falls back to the last 10 messages.
- Bot messages are sent as `assistant` turns. Everything else is a `user` turn with the author name prefixed.
- Context messages include UTC RFC3339 timestamps.
- The prompt marks earlier messages as reference only. The final user message is the request, and older context may be unrelated.
- The default system prompt tells the bot to reply like a peer, keep sentences simple, avoid em dashes, and stay under 280 characters unless asked for more. Override it with `CHAT_SYSTEM_PROMPT`.

## Web search

- The model searches the web when it decides it needs current information.
- Searches are capped by `OPENROUTER_WEB_SEARCH_MAX_USES`, 2 by default.
- Results per search are capped by `OPENROUTER_WEB_SEARCH_MAX_RESULTS`, 3 by default.
- Citations come back as inline Discord markdown links.
- The prompt asks for at most two citations unless the user asks for more, and forbids inventing URLs.
- Web search uses OpenRouter credits. The default Exa engine costs roughly $0.007 per search, plus model token costs.
- Disable it with `OPENROUTER_WEB_SEARCH=false`.

## Discord formatting

- Users are mentioned with `<@1234567890>`.
- Channels are mentioned with `<#1234567890>`.
- Timestamps use Unix seconds, for example `<t:1795222140:R>` or `<t:1795222140:f>`.
- The bot includes the current Unix time in each request so timestamps stay accurate.
- User mentions can ping. `@everyone`, `@here`, and role mentions are blocked.

## Images

- Attach an image to a mention or reply and the bot can see it.
- Images on the replied-to message are also included.
- Supported types: PNG, JPEG, WebP, and GIF.
- Default limits: `VISION_MAX_IMAGES` of 4 per request and `VISION_MAX_IMAGE_MB` of 8 MB per image.
- Only the current and replied-to message images are sent, not the whole channel history.
- Disable it with `VISION_ENABLED=false`.

## Limits

- Other bots and the bot's own messages are ignored.
- Direct messages are ignored. Server channels only for now.
- Replies and mentions are rate limited per user with `CHAT_RATE_LIMIT_CAPACITY` and `CHAT_RATE_LIMIT_SECONDS`.
- Concurrent model requests are capped by `CHAT_MAX_CONCURRENT`.
- Responses are split across at most `CHAT_MAX_CHUNKS` messages, 3 by default.

Messages in the context window are sent to OpenRouter. Do not mention the bot in channels where that is not acceptable.

## `/ask`

`/ask` uses the same pipeline as a mention, with two extra controls.

- `history_length` defaults to 10. It can be raised to 100.
- `disable_memory` skips both memory reads and memory writes for that ask.
- Responses are public.

```text
/ask text:what did we decide about the deploy? history_length:30
```
