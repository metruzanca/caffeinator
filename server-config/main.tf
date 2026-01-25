terraform {
  backend "s3" {
    bucket = "terraform-state-3abuno2jq"
    key    = "server-config/terraform.tfstate"
    region = "auto"

    # S3-compatible endpoint settings
    endpoints = {
      s3 = "https://storage.railway.app"
    }

    # Required for S3-compatible storage
    skip_credentials_validation = true
    skip_metadata_api_check     = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    use_path_style              = true
  }

  required_providers {
    discord = {
      source  = "Lucky3028/discord"
      version = "~> 2.3"
    }
  }
}

provider "discord" {
  # Token will be set via DISCORD_TOKEN environment variable
}

# Variables
variable "server_id" {
  description = "Discord server ID"
  type        = string
  default     = "1033770808360841317"
}

# Discord Server
resource "discord_server" "main" {
  name                          = "The Coffee Break;"
  region                        = "us-west"
  afk_timeout                   = 300
  default_message_notifications = 1
  explicit_content_filter       = 2
  verification_level            = 1
}

# =============================================================================
# Categories
# =============================================================================

resource "discord_category_channel" "checkpoint" {
  server_id = discord_server.main.id
  name      = "Checkpoint"
  position  = 0
}

resource "discord_category_channel" "main" {
  server_id = discord_server.main.id
  name      = "main"
  position  = 1
}

resource "discord_category_channel" "topics" {
  server_id = discord_server.main.id
  name      = "Topics"
  position  = 2
}

resource "discord_category_channel" "langs" {
  server_id = discord_server.main.id
  name      = "Langs"
  position  = 3
}

resource "discord_category_channel" "admin" {
  server_id = discord_server.main.id
  name      = "admin"
  position  = 4
}

resource "discord_category_channel" "archived" {
  server_id = discord_server.main.id
  name      = "Archived"
  position  = 5
}

# =============================================================================
# Text Channels (No Category)
# =============================================================================

resource "discord_text_channel" "info" {
  server_id                = discord_server.main.id
  name                     = "info"
  position                 = 0
  sync_perms_with_category = false
}

resource "discord_text_channel" "wassup" {
  server_id                = discord_server.main.id
  name                     = "wassup"
  position                 = 1
  sync_perms_with_category = false
  topic = chomp(<<-EOT
Introduce yourself to others!

wassup? https://youtu.be/lhUlKGmb1VY?t=32
EOT
  )
}

# =============================================================================
# Checkpoint Category Channels
# =============================================================================

resource "discord_text_channel" "checkpoint" {
  server_id                = discord_server.main.id
  name                     = "checkpoint"
  category                 = discord_category_channel.checkpoint.id
  position                 = 2
  sync_perms_with_category = false
  topic = chomp(<<-EOT
Every first and third Sunday of the month at 7PM UTC+0

https://time.is/0700PM_in_UTC?Checkpoint
EOT
  )
}

resource "discord_voice_channel" "checkpoint" {
  server_id                = discord_server.main.id
  name                     = "Checkpoint"
  category                 = discord_category_channel.checkpoint.id
  position                 = 0
  bitrate                  = 63688
  sync_perms_with_category = true
}

# =============================================================================
# Main Category Channels
# =============================================================================

resource "discord_text_channel" "general" {
  server_id                = discord_server.main.id
  name                     = "general"
  category                 = discord_category_channel.main.id
  position                 = 3
  sync_perms_with_category = true
}

resource "discord_text_channel" "memes" {
  server_id                = discord_server.main.id
  name                     = "memes"
  category                 = discord_category_channel.main.id
  position                 = 4
  sync_perms_with_category = true
}

resource "discord_text_channel" "share_tech" {
  server_id                = discord_server.main.id
  name                     = "share-tech"
  category                 = discord_category_channel.main.id
  position                 = 5
  sync_perms_with_category = true
  topic                    = "Found something new & cool? Share it here."
}

resource "discord_text_channel" "code_no_worky" {
  server_id                = discord_server.main.id
  name                     = "code-no-worky-pls-help"
  category                 = discord_category_channel.main.id
  position                 = 6
  sync_perms_with_category = true
}

resource "discord_voice_channel" "coffee_break" {
  server_id                = discord_server.main.id
  name                     = "Coffee Break"
  category                 = discord_category_channel.main.id
  position                 = 1
  bitrate                  = 64000
  sync_perms_with_category = true
}

# =============================================================================
# Topics Category Channels
# =============================================================================

resource "discord_text_channel" "music" {
  server_id                = discord_server.main.id
  name                     = "music"
  category                 = discord_category_channel.topics.id
  position                 = 7
  sync_perms_with_category = true
}

resource "discord_text_channel" "gaming" {
  server_id                = discord_server.main.id
  name                     = "gaming"
  category                 = discord_category_channel.topics.id
  position                 = 8
  sync_perms_with_category = true
}

resource "discord_text_channel" "gear" {
  server_id                = discord_server.main.id
  name                     = "gear"
  category                 = discord_category_channel.topics.id
  position                 = 9
  sync_perms_with_category = true
  topic = chomp(<<-EOT
Post and talk about your hardware setups.

Anything non-software is fair game.
EOT
  )
}

resource "discord_text_channel" "fitness" {
  server_id                = discord_server.main.id
  name                     = "fitness"
  category                 = discord_category_channel.topics.id
  position                 = 10
  sync_perms_with_category = true
}

resource "discord_text_channel" "obsidian" {
  server_id                = discord_server.main.id
  name                     = "obsidian"
  category                 = discord_category_channel.topics.id
  position                 = 11
  sync_perms_with_category = true
}

resource "discord_text_channel" "jank" {
  server_id                = discord_server.main.id
  name                     = "jank"
  category                 = discord_category_channel.topics.id
  position                 = 12
  sync_perms_with_category = true
  topic                    = "Something software wise piss you off? Rant about it here."
}

resource "discord_text_channel" "vim" {
  server_id                = discord_server.main.id
  name                     = "vim"
  category                 = discord_category_channel.topics.id
  position                 = 13
  sync_perms_with_category = true
}

resource "discord_text_channel" "cybersecurity" {
  server_id                = discord_server.main.id
  name                     = "cybersecurity"
  category                 = discord_category_channel.topics.id
  position                 = 14
  sync_perms_with_category = true
}

resource "discord_text_channel" "ai" {
  server_id                = discord_server.main.id
  name                     = "ai"
  category                 = discord_category_channel.topics.id
  position                 = 15
  sync_perms_with_category = true
}

resource "discord_text_channel" "gamedev" {
  server_id                = discord_server.main.id
  name                     = "gamedev"
  category                 = discord_category_channel.topics.id
  position                 = 16
  sync_perms_with_category = true
}

# =============================================================================
# Langs Category Channels
# =============================================================================

resource "discord_text_channel" "rust" {
  server_id                = discord_server.main.id
  name                     = "rust"
  category                 = discord_category_channel.langs.id
  position                 = 17
  sync_perms_with_category = true
}

resource "discord_text_channel" "go" {
  server_id                = discord_server.main.id
  name                     = "go"
  category                 = discord_category_channel.langs.id
  position                 = 18
  sync_perms_with_category = true
}

# =============================================================================
# Admin Category Channels
# =============================================================================

resource "discord_text_channel" "bot_abuse" {
  server_id                = discord_server.main.id
  name                     = "bot-abuse"
  category                 = discord_category_channel.admin.id
  position                 = 19
  sync_perms_with_category = true
}

resource "discord_text_channel" "moderator_only" {
  server_id                = discord_server.main.id
  name                     = "moderator-only"
  category                 = discord_category_channel.admin.id
  position                 = 20
  sync_perms_with_category = false
}

# =============================================================================
# Archived Category Channels
# =============================================================================

resource "discord_text_channel" "til" {
  server_id                = discord_server.main.id
  name                     = "til"
  category                 = discord_category_channel.archived.id
  position                 = 21
  sync_perms_with_category = true
  topic                    = "**T**oday **I** **L**earned | The place to share what you've been learning _and possibly spark discussion_."
}

resource "discord_text_channel" "gear_kbs" {
  server_id                = discord_server.main.id
  name                     = "gear-kbs"
  category                 = discord_category_channel.archived.id
  position                 = 22
  sync_perms_with_category = true
}

# =============================================================================
# Roles (non-managed only - managed roles like Dyno, ModMail, etc are excluded)
# =============================================================================

resource "discord_role" "sudoer" {
  server_id   = discord_server.main.id
  name        = "sudoer"
  permissions = 8 # Administrator
  position    = 14
  hoist       = false
  mentionable = false
}

resource "discord_role" "cron_weekly" {
  server_id   = discord_server.main.id
  name        = "cron-weekly"
  permissions = 1071698660929
  position    = 13
  hoist       = false
  mentionable = false
}

resource "discord_role" "bot" {
  server_id   = discord_server.main.id
  name        = "Bot"
  color       = 6768639 # #6747FF
  permissions = 1071698660929
  position    = 11
  hoist       = true
  mentionable = false
}

resource "discord_role" "forced_to_use_php" {
  server_id   = discord_server.main.id
  name        = "Forced to use PHP"
  color       = 7830451 # #777BB3
  permissions = 0
  position    = 10
  hoist       = false
  mentionable = true
}

resource "discord_role" "edgy_cunt" {
  server_id   = discord_server.main.id
  name        = "Edgy Cunt"
  color       = 16727791 # #FF3EEF
  permissions = 0
  position    = 9
  hoist       = false
  mentionable = false
}

resource "discord_role" "arch_btw" {
  server_id   = discord_server.main.id
  name        = "Arch BTW"
  color       = 59903 # #00E9FF
  permissions = 0
  position    = 8
  hoist       = true
  mentionable = true
}

resource "discord_role" "grug_debian_user" {
  server_id   = discord_server.main.id
  name        = "Grug Debian User"
  color       = 13567291 # #CF053B
  permissions = 0
  position    = 7
  hoist       = false
  mentionable = false
}

resource "discord_role" "filthy_windows_user" {
  server_id   = discord_server.main.id
  name        = "Filthy Windows User"
  color       = 1476602 # #1687FA
  permissions = 0
  position    = 6
  hoist       = true
  mentionable = true
}

resource "discord_role" "checkpoint" {
  server_id   = discord_server.main.id
  name        = "Checkpoint"
  color       = 2067276 # #1F8B4C
  permissions = 0
  position    = 5
  hoist       = false
  mentionable = false
}

resource "discord_role" "matts_accounts" {
  server_id   = discord_server.main.id
  name        = "Matt's Accounts"
  permissions = 1071698660929
  position    = 4
  hoist       = false
  mentionable = true
}

resource "discord_role" "mr_emails" {
  server_id   = discord_server.main.id
  name        = "Mr Emails"
  permissions = 0
  position    = 1
  hoist       = false
  mentionable = true
}
