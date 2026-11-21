package commands

import (
	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
)

// interactionUserID returns the user who invoked an interaction.
func interactionUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

// respondEphemeral sends a private response to an interaction.
func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
			AllowedMentions: &discordgo.MessageAllowedMentions{
				Parse:       []discordgo.AllowedMentionType{},
				RepliedUser: false,
			},
		},
	}); err != nil {
		log.Error("cannot respond to interaction", "err", err, "channel", i.ChannelID)
	}
}
