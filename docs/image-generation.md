# Image generation

`/imagine` generates images through OpenRouter. The default model is `black-forest-labs/flux-3-image-klein`.

```text
/imagine prompt:a red panda astronaut, studio lighting
/imagine prompt:turn this into a watercolor painting image:<attachment>
```

Options:

- `image` is an optional reference image for editing or style transfer.
- `aspect_ratio` supports `1:1`, `16:9`, `9:16`, `4:3`, `3:4`, `3:2`, `2:3`, and `auto`.
- `resolution` supports `1K`, `1.5K`, and `2K`.
- `safety_tolerance` supports 0 through 4 in every channel.

Defaults:

- Aspect ratio `1:1`
- Resolution `1K`
- Safety tolerance `2`
- One image per request
- `IMAGE_RATE_LIMIT_PER_HOUR` generations per user per hour, 20 by default

A higher tolerance does not guarantee mature output. Black Forest Labs still blocks illegal content.

Reference images are downloaded from Discord, converted to base64, and sent to OpenRouter. PNG, JPEG, and WebP are accepted. The default limit is 8 MB.

Image generation is billed per image by OpenRouter, not by tokens. Flux 3 Image Klein costs about $0.009 per image at 1K at the time of writing.

If the provider blocks a prompt, the bot shows the provider reason. If generation is rate limited, the bot replies with a Discord relative timestamp. This covers both the per-user limit and provider throttling. When a prompt fails, the reply ends with a copy-pasteable retry command:

```text
Try again with:
/imagine prompt:a duck
```

Relevant settings: `IMAGE_ENABLED`, `IMAGE_MODEL`, `IMAGE_DEFAULT_RESOLUTION`, `IMAGE_DEFAULT_ASPECT_RATIO`, `IMAGE_SAFETY_TOLERANCE`, `IMAGE_MAX_REFERENCE_MB`, `IMAGE_TIMEOUT_SECONDS`, and `IMAGE_RATE_LIMIT_PER_HOUR`.
