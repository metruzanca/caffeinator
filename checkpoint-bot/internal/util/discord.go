package util

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

// GetInviteLink returns an OAuth2 URL with the permissions the bot needs.
func GetInviteLink(botID string) string {
	permissions := int64(discordgo.PermissionViewChannel) |
		int64(discordgo.PermissionSendMessages) |
		int64(discordgo.PermissionSendMessagesInThreads) |
		int64(discordgo.PermissionReadMessageHistory) |
		int64(discordgo.PermissionAttachFiles)

	return fmt.Sprintf(
		"https://discord.com/api/oauth2/authorize?client_id=%s&permissions=%d&scope=bot%%20applications.commands",
		botID,
		permissions,
	)
}
