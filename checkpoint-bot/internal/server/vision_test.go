package server

import (
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/require"
)

func visionChat(t *testing.T, maxImages, maxMB int) *Chat {
	t.Helper()
	return newChat(&fakeChatSession{}, &fakeChatClient{}, ChatConfig{
		VisionEnabled:    true,
		VisionMaxImages:  maxImages,
		VisionMaxImageMB: maxMB,
		VisionDetail:     "auto",
	})
}

func TestVisionPartsIncludesCurrentAndReferencedImages(t *testing.T) {
	chat := visionChat(t, 4, 8)
	current := testMessage("1", "channel", "guild", "111", "what is this?")
	current.Attachments = []*discordgo.MessageAttachment{
		{
			Filename:    "current.png",
			ContentType: "image/png",
			Size:        1000,
			URL:         "https://cdn.discordapp.com/attachments/1/2/current.png",
		},
	}
	referenced := &discordgo.Message{
		ID: "ref",
		Attachments: []*discordgo.MessageAttachment{
			{
				Filename:    "referenced.jpg",
				ContentType: "image/jpeg",
				Size:        2000,
				URL:         "https://media.discordapp.net/attachments/3/4/referenced.jpg",
			},
		},
	}

	parts := chat.visionParts(current, referenced)

	require.Len(t, parts, 4)
	require.Equal(t, "text", parts[0].Type)
	require.Contains(t, parts[0].Text, "current.png")
	require.Equal(t, "image_url", parts[1].Type)
	require.Equal(t, "https://cdn.discordapp.com/attachments/1/2/current.png", parts[1].ImageURL.URL)
	require.Equal(t, "auto", parts[1].ImageURL.Detail)
	require.Contains(t, parts[2].Text, "referenced.jpg")
	require.Equal(t, "https://media.discordapp.net/attachments/3/4/referenced.jpg", parts[3].ImageURL.URL)
}

func TestVisionPartsFiltersUnsupportedAndOversizedImages(t *testing.T) {
	chat := visionChat(t, 2, 1)
	current := testMessage("1", "channel", "guild", "111", "look")
	current.Attachments = []*discordgo.MessageAttachment{
		{
			Filename:    "notes.txt",
			ContentType: "text/plain",
			Size:        100,
			URL:         "https://cdn.discordapp.com/attachments/1/2/notes.txt",
		},
		{
			Filename:    "huge.png",
			ContentType: "image/png",
			Size:        2 * 1024 * 1024,
			URL:         "https://cdn.discordapp.com/attachments/1/2/huge.png",
		},
		{
			Filename:    "external.png",
			ContentType: "image/png",
			Size:        100,
			URL:         "https://example.com/external.png",
		},
		{
			Filename:    "ok.webp",
			ContentType: "image/webp",
			Size:        100,
			URL:         "https://cdn.discordapp.com/attachments/1/2/ok.webp",
		},
	}

	parts := chat.visionParts(current, nil)

	require.Len(t, parts, 2)
	require.Contains(t, parts[0].Text, "ok.webp")
	require.Equal(t, "https://cdn.discordapp.com/attachments/1/2/ok.webp", parts[1].ImageURL.URL)
}

func TestVisionPartsRespectsMaxImages(t *testing.T) {
	chat := visionChat(t, 1, 8)
	current := testMessage("1", "channel", "guild", "111", "look")
	current.Attachments = []*discordgo.MessageAttachment{
		{
			Filename:    "one.png",
			ContentType: "image/png",
			Size:        100,
			URL:         "https://cdn.discordapp.com/attachments/1/2/one.png",
		},
		{
			Filename:    "two.png",
			ContentType: "image/png",
			Size:        100,
			URL:         "https://cdn.discordapp.com/attachments/1/2/two.png",
		},
	}

	parts := chat.visionParts(current, nil)

	require.Len(t, parts, 2)
	require.Contains(t, parts[0].Text, "one.png")
	require.NotContains(t, parts[0].Text, "two.png")
}

func TestVisionPartsDisabled(t *testing.T) {
	chat := newChat(&fakeChatSession{}, &fakeChatClient{}, ChatConfig{})
	current := testMessage("1", "channel", "guild", "111", "look")
	current.Attachments = []*discordgo.MessageAttachment{{
		Filename:    "image.png",
		ContentType: "image/png",
		Size:        100,
		URL:         "https://cdn.discordapp.com/attachments/1/2/image.png",
	}}

	require.Empty(t, chat.visionParts(current, nil))
}
