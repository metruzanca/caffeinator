# Discord Server Discovery

[disc](https://github.com/metruzanca/disc) is installed via mise (see `mise.toml`) and can be used to discover Discord server structure for importing into terraform.

## Environment Variables

- `DISCORD_TOKEN` - Bot token (same as terraform provider)
- `DISCORD_SERVER_ID` - Server/guild ID to query

## Usage

Use `railway run` to access the environment variables:

```bash
# List channels (for terraform import)
railway run disc channel list

# List roles (for terraform import)
railway run disc role list
```

The output can be used with `terraform import` to import existing channels and roles into the terraform state.