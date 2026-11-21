package commands

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
	"github.com/metruzanca/checkpoint-bot/internal/imagegen"
)

// ImagineCmd generates images with OpenRouter.
var ImagineCmd = &Command{
	ApplicationCommand: discordgo.ApplicationCommand{
		Name:        "imagine",
		Description: "Generate an image, optionally using an attached reference image",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "prompt",
				Description: "What to generate or change",
				Required:    true,
				MaxLength:   1000,
			},
			{
				Type:        discordgo.ApplicationCommandOptionAttachment,
				Name:        "image",
				Description: "Optional reference image for editing or style transfer",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "aspect_ratio",
				Description: "Output aspect ratio",
				Required:    false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "1:1", Value: "1:1"},
					{Name: "16:9", Value: "16:9"},
					{Name: "9:16", Value: "9:16"},
					{Name: "4:3", Value: "4:3"},
					{Name: "3:4", Value: "3:4"},
					{Name: "3:2", Value: "3:2"},
					{Name: "2:3", Value: "2:3"},
					{Name: "auto", Value: "auto"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "resolution",
				Description: "Output resolution",
				Required:    false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "1K", Value: "1K"},
					{Name: "1.5K", Value: "1.5K"},
					{Name: "2K", Value: "2K"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "safety_tolerance",
				Description: "Moderation tolerance, 0 strict to 5 permissive. NSFW channels only",
				Required:    false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "0 strictest", Value: 0},
					{Name: "1", Value: 1},
					{Name: "2 default", Value: 2},
					{Name: "3", Value: 3},
					{Name: "4", Value: 4},
					{Name: "5 most permissive", Value: 5},
				},
			},
		},
	},
	Handler: func(h *CommandHandler, s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h.Images == nil {
			respondEphemeral(s, i, "Image generation is not configured.")
			return
		}

		userID := interactionUserID(i)
		if h.ImageLimiter != nil && !h.ImageLimiter.Allow(userID) {
			respondEphemeral(s, i, "You're generating images too quickly. Try again later.")
			return
		}

		var prompt string
		aspectRatio := h.ImageAspectRatio
		resolution := h.ImageResolution
		var requestedTolerance *int
		var attachment *discordgo.MessageAttachment

		data := i.ApplicationCommandData()
		for _, opt := range data.Options {
			switch opt.Name {
			case "prompt":
				prompt = strings.TrimSpace(opt.StringValue())
			case "aspect_ratio":
				aspectRatio = opt.StringValue()
			case "resolution":
				resolution = opt.StringValue()
			case "safety_tolerance":
				value := int(opt.IntValue())
				requestedTolerance = &value
			case "image":
				id, ok := opt.Value.(string)
				if ok && data.Resolved != nil {
					attachment = data.Resolved.Attachments[id]
				}
			}
		}

		if prompt == "" {
			respondEphemeral(s, i, "Prompt cannot be empty.")
			return
		}

		nsfw, err := channelIsNSFW(s, i.ChannelID)
		if err != nil {
			log.Warn("cannot check channel NSFW status", "err", err, "channel", i.ChannelID)
			nsfw = false
		}
		tolerance, err := resolveImageSafetyTolerance(nsfw, requestedTolerance, h.ImageSafeTolerance, h.ImageNSFWTolerance)
		if err != nil {
			respondEphemeral(s, i, "Safety tolerance can only be changed in NSFW channels.")
			return
		}

		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		}); err != nil {
			log.Error("cannot defer imagine interaction", "err", err, "channel", i.ChannelID)
			return
		}

		timeout := h.ImageTimeout
		if timeout <= 0 {
			timeout = 2 * time.Minute
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		reference, err := downloadReferenceImage(ctx, attachment, h.ImageMaxReferenceBytes)
		if err != nil {
			log.Error("cannot download reference image", "err", err, "user", userID, "channel", i.ChannelID)
			editInteractionError(s, i, "Could not read that image. Use a PNG, JPEG, or WebP under the size limit.")
			return
		}

		started := time.Now()
		result, err := h.Images.Generate(ctx, imagegen.Request{
			Prompt:           prompt,
			Resolution:       resolution,
			AspectRatio:      aspectRatio,
			ReferenceDataURL: reference,
			SafetyTolerance:  &tolerance,
		})
		if err != nil {
			log.Error("image generation failed", "err", err, "user", userID, "channel", i.ChannelID)
			editInteractionError(s, i, imageGenerationErrorMessage(err, nsfw))
			return
		}

		log.Info("image generated",
			"user", userID,
			"channel", i.ChannelID,
			"model", h.ImageModel,
			"images", len(result.Images),
			"cost_usd", result.CostUSD,
			"duration", time.Since(started),
			"reference", attachment != nil,
			"safety_tolerance", tolerance,
			"nsfw_channel", nsfw,
		)

		if err := sendGeneratedImages(s, i, prompt, result.Images); err != nil {
			log.Error("cannot send generated images", "err", err, "channel", i.ChannelID)
			editInteractionError(s, i, "Generated the image but could not upload it.")
		}
	},
}

func downloadReferenceImage(ctx context.Context, attachment *discordgo.MessageAttachment, maxBytes int64) (string, error) {
	if attachment == nil {
		return "", nil
	}
	if maxBytes <= 0 {
		maxBytes = 8 << 20
	}
	if attachment.Size > 0 && int64(attachment.Size) > maxBytes {
		return "", fmt.Errorf("reference image is too large")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, attachment.URL, nil)
	if err != nil {
		return "", fmt.Errorf("cannot create reference image request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot download reference image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("reference image download failed with status %d", resp.StatusCode)
	}

	contentType := strings.TrimSpace(strings.Split(attachment.ContentType, ";")[0])
	if contentType == "" {
		contentType = strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	}
	if !supportedReferenceType(contentType) {
		return "", fmt.Errorf("unsupported reference image type %q", contentType)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("cannot read reference image: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return "", fmt.Errorf("reference image is too large")
	}

	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func supportedReferenceType(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/webp":
		return true
	default:
		return false
	}
}

func sendGeneratedImages(s *discordgo.Session, i *discordgo.InteractionCreate, prompt string, images []imagegen.Output) error {
	if len(images) == 0 {
		return fmt.Errorf("no images to send")
	}

	caption := imageCaption(prompt)
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &caption,
		Files:   imageFiles(images),
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse: []discordgo.AllowedMentionType{},
		},
	})
	if err == nil {
		return nil
	}

	log.Warn("cannot attach image to deferred response, using follow-up", "err", err)
	message := caption
	if _, editErr := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &message,
	}); editErr != nil {
		return editErr
	}
	_, err = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Files: imageFiles(images),
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse: []discordgo.AllowedMentionType{},
		},
	})
	return err
}

// imageCaption uses the prompt as the message text shown with the images.
func imageCaption(prompt string) string {
	return strings.TrimSpace(prompt)
}

func imageFiles(images []imagegen.Output) []*discordgo.File {
	files := make([]*discordgo.File, 0, len(images))
	for index, image := range images {
		files = append(files, &discordgo.File{
			Name:        fmt.Sprintf("imagine-%d%s", index+1, imageExtension(image.MediaType)),
			ContentType: image.MediaType,
			Reader:      bytes.NewReader(image.Data),
		})
	}
	return files
}

func imageGenerationErrorMessage(err error, nsfw bool) string {
	var blocked *imagegen.ContentBlockedError
	if errors.As(err, &blocked) {
		if nsfw {
			reason := strings.TrimSpace(blocked.Reason)
			if reason == "" {
				reason = "content policy"
			}
			return "Blocked by the image provider: " + reason
		}
		if blocked.Category == imagegen.BlockedSexual {
			return "That's a bit too lewd for my liking. This is a Christian server."
		}
		return "The image provider blocked that prompt."
	}
	return "Image generation failed. Try again in a bit."
}

func channelIsNSFW(s *discordgo.Session, channelID string) (bool, error) {
	channel, err := s.Channel(channelID)
	if err != nil {
		return false, err
	}
	if channel.NSFW {
		return true, nil
	}
	if channel.ParentID != "" {
		parent, err := s.Channel(channel.ParentID)
		if err == nil && parent.NSFW {
			return true, nil
		}
	}
	return false, nil
}

func resolveImageSafetyTolerance(nsfw bool, requested *int, safeDefault, nsfwDefault int) (int, error) {
	if requested != nil {
		if !nsfw {
			return 0, fmt.Errorf("safety tolerance can only be changed in NSFW channels")
		}
		return clampTolerance(*requested), nil
	}
	if nsfw {
		return clampTolerance(nsfwDefault), nil
	}
	return clampTolerance(safeDefault), nil
}

func clampTolerance(value int) int {
	if value < 0 {
		return 0
	}
	if value > 5 {
		return 5
	}
	return value
}

func imageExtension(mediaType string) string {
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/svg+xml":
		return ".svg"
	default:
		return ".png"
	}
}

func init() {
	registerCommand(ImagineCmd)
}
