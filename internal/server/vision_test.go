package server

import (
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/require"
)

func visionChat(t *testing.T, maxImages, maxMB int) *Chat {
	t.Helper()
	return newChat(&fakeChatSession{}, &fakeAgent{}, ChatConfig{
		VisionEnabled:    true,
		VisionMaxImages:  maxImages,
		VisionMaxImageMB: maxMB,
		VisionDetail:     "auto",
	})
}

func TestVisionImagesIncludesCurrentAndReferencedImages(t *testing.T) {
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

	images := chat.visionImages(current, referenced)

	require.Len(t, images, 2)
	require.Contains(t, images[0].Label, "current.png")
	require.Equal(t, "https://cdn.discordapp.com/attachments/1/2/current.png", images[0].URL)
	require.Equal(t, "auto", images[0].Detail)
	require.Contains(t, images[1].Label, "referenced.jpg")
	require.Equal(t, "https://media.discordapp.net/attachments/3/4/referenced.jpg", images[1].URL)
}

func TestVisionImagesFiltersUnsupportedAndOversizedImages(t *testing.T) {
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

	images := chat.visionImages(current, nil)

	require.Len(t, images, 1)
	require.Contains(t, images[0].Label, "ok.webp")
	require.Equal(t, "https://cdn.discordapp.com/attachments/1/2/ok.webp", images[0].URL)
}

func TestVisionImagesRespectsMaxImages(t *testing.T) {
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

	images := chat.visionImages(current, nil)

	require.Len(t, images, 1)
	require.Contains(t, images[0].Label, "one.png")
	require.NotContains(t, images[0].Label, "two.png")
}

func TestVisionImagesDisabled(t *testing.T) {
	chat := newChat(&fakeChatSession{}, &fakeAgent{}, ChatConfig{})
	current := testMessage("1", "channel", "guild", "111", "look")
	current.Attachments = []*discordgo.MessageAttachment{{
		Filename:    "image.png",
		ContentType: "image/png",
		Size:        100,
		URL:         "https://cdn.discordapp.com/attachments/1/2/image.png",
	}}

	require.Empty(t, chat.visionImages(current, nil))
}
