package server

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
	"github.com/metruzanca/checkpoint-bot/internal/llm"
)

func (c *Chat) visionParts(current *discordgo.MessageCreate, referenced *discordgo.Message) []llm.ContentPart {
	if !c.cfg.VisionEnabled {
		return nil
	}

	maxImages := c.cfg.VisionMaxImages
	maxBytes := int64(c.cfg.VisionMaxImageMB) * 1024 * 1024
	images := 0

	parts := make([]llm.ContentPart, 0, maxImages*2)
	add := func(attachments []*discordgo.MessageAttachment, label string) {
		for _, attachment := range attachments {
			if images >= maxImages {
				return
			}
			if attachment == nil {
				continue
			}
			if !supportedImageType(attachment.ContentType) {
				log.Debug("skipping unsupported image attachment", "filename", attachment.Filename, "type", attachment.ContentType)
				continue
			}
			if attachment.Size <= 0 || int64(attachment.Size) > maxBytes {
				log.Debug("skipping oversized image attachment", "filename", attachment.Filename, "size", attachment.Size)
				continue
			}

			imageURL := attachmentURL(attachment)
			if imageURL == "" {
				log.Debug("skipping image attachment without a Discord URL", "filename", attachment.Filename)
				continue
			}

			parts = append(parts, llm.ContentPart{
				Type: "text",
				Text: fmt.Sprintf("%s: %s", label, attachment.Filename),
			})
			parts = append(parts, llm.ContentPart{
				Type: "image_url",
				ImageURL: &llm.ImageURL{
					URL:    imageURL,
					Detail: c.cfg.VisionDetail,
				},
			})
			images++
		}
	}

	if current != nil {
		add(current.Attachments, "Current message image")
	}
	if referenced != nil {
		add(referenced.Attachments, "Referenced message image")
	}

	return parts
}

func supportedImageType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
	case "image/png", "image/jpeg", "image/jpg", "image/webp", "image/gif":
		return true
	default:
		return false
	}
}

func attachmentURL(attachment *discordgo.MessageAttachment) string {
	for _, candidate := range []string{attachment.URL, attachment.ProxyURL} {
		if candidate == "" {
			continue
		}
		parsed, err := url.Parse(candidate)
		if err != nil || parsed.Scheme != "https" {
			continue
		}
		host := strings.ToLower(parsed.Hostname())
		if strings.HasSuffix(host, ".discordapp.com") || strings.HasSuffix(host, ".discordapp.net") {
			return candidate
		}
	}
	return ""
}
