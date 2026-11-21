# AGENTS.md

## Rules

- Use conventional commits (`feat:`, `fix:`, `chore:`, ...) and commit in small, frequent increments.
- never leave uncommitted changes
- once finished implementing a feature, push to main to trigger a new deploy
- never use em dashes. Keep sentences short and simple.

## Structure

- Caffeinator: a self-hostable Discord bot powered by an LLM with memory and web search. The repository root is the Go module.
- Keep the project generic. Do not hardcode The Coffee Break in code. Use `BOT_NAME` and `COMMUNITY_NAME` for branding.
