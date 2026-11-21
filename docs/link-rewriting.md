# Link rewriting

`/link` rewrites social media links so Discord can embed them. It is a pure URL rewrite and never fetches the link, which avoids SSRF.

```text
/link url:https://x.com/4ster_light/status/2116158570198622382?s=20
→ https://fxtwitter.com/4ster_light/status/2116158570198622382

/link url:https://www.instagram.com/p/ABC123/?igsh=tracking
→ https://kkinstagram.com/p/ABC123
```

Rules:

- Rewrites only allowlisted hosts.
- Forces HTTPS.
- Removes query strings and fragments.
- Preserves the path.
- Accepts already rewritten links.

Supported hosts: `x.com`, `twitter.com`, `mobile.x.com`, `mobile.twitter.com`, `instagram.com`, `m.instagram.com`, `fxtwitter.com`, and `kkinstagram.com`.

The rewritten link is posted publicly so everyone can click or preview it. The command does not need OpenRouter.
