package server

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
	"github.com/metruzanca/checkpoint-bot/internal/ratelimit"
	"github.com/metruzanca/checkpoint-bot/internal/reminders"
)

var (
	joinPattern            = regexp.MustCompile(`(?i)^\s*(me too|count me in|i'?m in|im in|add me|join|same|\+1)(\s+please)?\s*[.!]*\s*$`)
	leavePattern           = regexp.MustCompile(`(?i)^\s*(leave|remove me|don'?t include me|not me)(\s+please)?\s*[.!]*\s*$`)
	reminderKeywordPattern = regexp.MustCompile(`(?i)\b(remind|reminder|reminders|event|events|schedule|every|daily|weekly|monthly|tomorrow|today|tonight|next)\b`)
)

// ReminderService connects reminders to Discord.
type ReminderService struct {
	session    *discordgo.Session
	store      *reminders.Store
	parser     *reminders.Parser
	writer     *reminders.Writer
	limiter    *ratelimit.Limiter
	maxPerUser int
}

// NewReminderService creates the Discord reminder service.
func NewReminderService(session *discordgo.Session, store *reminders.Store, parser *reminders.Parser, writer *reminders.Writer, maxPerUser int) *ReminderService {
	return &ReminderService{
		session:    session,
		store:      store,
		parser:     parser,
		writer:     writer,
		limiter:    ratelimit.New(5, time.Minute),
		maxPerUser: maxPerUser,
	}
}

// HandleMessage handles joins and natural language reminder requests.
// It returns true when the message was handled.
func (c *ReminderService) HandleMessage(ctx context.Context, m *discordgo.MessageCreate) bool {
	if c == nil || c.session == nil || c.session.State == nil || c.session.State.User == nil {
		return false
	}
	if m == nil || m.Message == nil || m.Author == nil || m.Author.Bot || m.GuildID == "" {
		return false
	}

	if c.handleControl(m) {
		return true
	}

	botID := c.session.State.User.ID
	if !mentionsUser(m, botID) {
		return false
	}

	text := cleanContent(m, botID, true)
	if !looksLikeReminderRequest(text) {
		return false
	}
	if !c.limiter.Allow(m.Author.ID) {
		c.sendReply(m, "You're creating reminders too quickly. Try again in a minute.")
		return true
	}

	timezone := c.store.Timezone(m.Author.ID)
	now := time.Now()
	if timezone != "" {
		if loc, err := time.LoadLocation(timezone); err == nil {
			now = now.In(loc)
		}
	}
	parseCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := c.parser.Parse(parseCtx, text, timezone, now)
	if err != nil {
		log.Error("cannot parse reminder request", "err", err, "user", m.Author.ID, "channel", m.ChannelID)
		c.sendReply(m, "I couldn't parse that reminder. Try something like: remind me tomorrow at 4pm.")
		return true
	}
	if result.Confidence < 0.5 {
		c.sendReply(m, "I'm not sure what you meant. Try: remind me tomorrow at 4pm.")
		return true
	}

	switch result.Action {
	case "list":
		c.sendList(m)
		return true
	case "create":
		c.createReminder(m, result)
		return true
	default:
		return false
	}
}

func (c *ReminderService) createReminder(m *discordgo.MessageCreate, result reminders.ParseResult) {
	timezone := c.store.Timezone(m.Author.ID)
	if timezone == "" {
		c.sendReply(m, "Set your timezone first with /timezone. Example: /timezone zone:America/New_York")
		return
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		c.sendReply(m, "Your saved timezone is invalid. Set it again with /timezone.")
		return
	}

	start, err := parseStart(result.Start, loc)
	if err != nil {
		c.sendReply(m, "When should I remind you? Try: remind me tomorrow at 4pm.")
		return
	}
	if err := reminders.ValidateRecurrence(result.Recurrence); err != nil {
		c.sendReply(m, "I couldn't understand the repeat schedule. Try: every Monday at 9am.")
		return
	}

	now := time.Now()
	if result.Recurrence == nil && !start.After(now) {
		c.sendReply(m, "That time is in the past. Pick a future time.")
		return
	}

	next, ok := reminders.NextRun(now, start, result.Recurrence, loc)
	if !ok {
		c.sendReply(m, "I couldn't find a future time for that reminder.")
		return
	}

	if c.maxPerUser > 0 && c.store.CountCreatedBy(m.Author.ID) >= c.maxPerUser {
		c.sendReply(m, fmt.Sprintf("You already have %d active reminders. Cancel one with /reminders first.", c.maxPerUser))
		return
	}

	kind := reminders.Kind(result.Kind)
	if kind != reminders.KindEvent {
		kind = reminders.KindReminder
	}

	reminder := &reminders.Reminder{
		Kind:         kind,
		CreatorID:    m.Author.ID,
		GuildID:      m.GuildID,
		ChannelID:    m.ChannelID,
		Text:         strings.TrimSpace(result.Text),
		Timezone:     timezone,
		StartAt:      start.UTC(),
		NextRunAt:    next,
		Recurrence:   result.Recurrence,
		Active:       true,
		Participants: []string{},
	}
	if err := c.store.Create(reminder); err != nil {
		log.Error("cannot create reminder", "err", err, "user", m.Author.ID, "channel", m.ChannelID)
		c.sendReply(m, "I couldn't save that reminder. Try again in a bit.")
		return
	}

	message, err := c.sendAnnouncement(m, reminder, loc)
	if err != nil {
		log.Error("cannot send reminder announcement", "err", err, "id", reminder.ID, "channel", m.ChannelID)
		_ = c.store.Delete(reminder.ID)
		c.sendReply(m, "I couldn't post the reminder announcement. Try again in a bit.")
		return
	}

	reminder.AnnouncementMessageID = message.ID
	if err := c.store.Update(reminder); err != nil {
		log.Error("cannot save reminder announcement ID", "err", err, "id", reminder.ID)
	}
	log.Info("reminder created",
		"id", reminder.ID,
		"kind", reminder.Kind,
		"user", m.Author.ID,
		"channel", m.ChannelID,
		"next_run_at", reminder.NextRunAt,
		"recurring", reminder.Recurrence != nil,
	)
}

func (c *ReminderService) sendAnnouncement(m *discordgo.MessageCreate, reminder *reminders.Reminder, loc *time.Location) (*discordgo.Message, error) {
	when := reminder.NextRunAt.In(loc)
	label := "Reminder"
	if reminder.Kind == reminders.KindEvent {
		label = "Event"
	}

	var builder strings.Builder
	if reminder.Text != "" {
		fmt.Fprintf(&builder, "%s: %s\n", label, truncateRunes(reminder.Text, 500))
	} else {
		fmt.Fprintf(&builder, "%s\n", label)
	}
	if reminder.Recurrence == nil {
		fmt.Fprintf(&builder, "Scheduled for <t:%d:f> (<t:%d:R>).\n", when.Unix(), when.Unix())
	} else {
		fmt.Fprintf(&builder, "Next: <t:%d:f> (<t:%d:R>).\n", when.Unix(), when.Unix())
	}
	if reminder.Kind == reminders.KindEvent {
		builder.WriteString("Anyone can join by replying \"me too\".")
	} else {
		builder.WriteString("Reply with \"me too\" to join.")
	}

	failIfNotExists := false
	return c.session.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: builder.String(),
		Reference: &discordgo.MessageReference{
			MessageID:       m.ID,
			ChannelID:       m.ChannelID,
			GuildID:         m.GuildID,
			FailIfNotExists: &failIfNotExists,
		},
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse:       []discordgo.AllowedMentionType{},
			RepliedUser: false,
		},
	})
}

func (c *ReminderService) handleControl(m *discordgo.MessageCreate) bool {
	messageID := ""
	if m.ReferencedMessage != nil {
		messageID = m.ReferencedMessage.ID
	} else if m.MessageReference != nil {
		messageID = m.MessageReference.MessageID
	}
	if messageID == "" {
		return false
	}

	reminder, ok := c.store.FindByMessageID(messageID)
	if !ok {
		return false
	}

	content := strings.TrimSpace(m.Content)
	if joinPattern.MatchString(content) {
		if !reminder.Active && reminder.Recurrence == nil {
			c.react(m, "❌")
			return true
		}
		if reminder.CreatorID == m.Author.ID {
			c.react(m, "✅")
			return true
		}
		_, _, err := c.store.AddParticipant(reminder.ID, m.Author.ID)
		if err != nil {
			log.Error("cannot add reminder participant", "err", err, "id", reminder.ID, "user", m.Author.ID)
			c.react(m, "❌")
			return true
		}
		c.react(m, "✅")
		return true
	}

	if leavePattern.MatchString(content) {
		_, removed, err := c.store.RemoveParticipant(reminder.ID, m.Author.ID)
		if err != nil {
			c.react(m, "❌")
			return true
		}
		if removed {
			c.react(m, "👋")
		} else {
			c.react(m, "❌")
		}
		return true
	}

	return false
}

func (c *ReminderService) sendList(m *discordgo.MessageCreate) {
	items := c.store.ListForUser(m.Author.ID)
	c.sendReply(m, formatReminderList(items))
}

// ListForUser returns an ephemeral-friendly reminder list.
func (c *ReminderService) ListForUser(userID string) string {
	return formatReminderList(c.store.ListForUser(userID))
}

// SetTimezone saves a user's IANA timezone.
func (c *ReminderService) SetTimezone(userID, zone string) error {
	if _, err := time.LoadLocation(zone); err != nil {
		return err
	}
	return c.store.SetTimezone(userID, zone)
}

// Cancel deletes a reminder created by the user.
func (c *ReminderService) Cancel(userID, idPrefix string) (bool, error) {
	reminder, ok := c.store.FindByIDPrefix(idPrefix)
	if !ok || reminder.CreatorID != userID {
		return false, nil
	}
	return true, c.store.Delete(reminder.ID)
}

// Leave removes the user from a reminder.
func (c *ReminderService) Leave(userID, idPrefix string) (bool, error) {
	reminder, ok := c.store.FindByIDPrefix(idPrefix)
	if !ok {
		return false, nil
	}
	_, removed, err := c.store.RemoveParticipant(reminder.ID, userID)
	return removed, err
}

// Trigger sends one reminder message.
func (c *ReminderService) Trigger(ctx context.Context, reminder *reminders.Reminder) error {
	ids := reminder.MentionIDs()
	mentions := make([]string, 0, len(ids))
	for _, id := range ids {
		mentions = append(mentions, "<@"+id+">")
	}

	loc, err := time.LoadLocation(reminder.Timezone)
	if err != nil {
		loc = time.UTC
	}

	body := ""
	if c.writer != nil {
		writeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		body, err = c.writer.Write(writeCtx, reminder.Text, reminder.NextRunAt.In(loc))
		cancel()
		if err != nil {
			log.Warn("cannot generate reminder body", "err", err, "id", reminder.ID)
			body = ""
		}
	}
	body = strings.TrimSpace(body)
	if body == "" {
		if reminder.Text != "" {
			body = "Reminder: " + reminder.Text
		} else {
			body = "Reminder time."
		}
	}

	content := strings.Join(mentions, " ")
	if content != "" {
		content += " "
	}
	content = truncateRunes(content+body, 1900)

	data := &discordgo.MessageSend{
		Content: content,
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse:       []discordgo.AllowedMentionType{discordgo.AllowedMentionTypeUsers},
			Users:       ids,
			RepliedUser: false,
		},
	}
	if reminder.AnnouncementMessageID != "" {
		failIfNotExists := false
		data.Reference = &discordgo.MessageReference{
			MessageID:       reminder.AnnouncementMessageID,
			ChannelID:       reminder.ChannelID,
			GuildID:         reminder.GuildID,
			FailIfNotExists: &failIfNotExists,
		}
	}

	message, err := c.session.ChannelMessageSendComplex(reminder.ChannelID, data)
	if err != nil {
		return err
	}

	reminder.LastTriggerMessageID = message.ID
	if err := c.store.Update(reminder); err != nil {
		return err
	}
	log.Info("reminder triggered", "id", reminder.ID, "channel", reminder.ChannelID, "participants", len(ids))
	return nil
}

func (c *ReminderService) sendReply(m *discordgo.MessageCreate, content string) {
	failIfNotExists := false
	_, err := c.session.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: content,
		Reference: &discordgo.MessageReference{
			MessageID:       m.ID,
			ChannelID:       m.ChannelID,
			GuildID:         m.GuildID,
			FailIfNotExists: &failIfNotExists,
		},
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse:       []discordgo.AllowedMentionType{},
			RepliedUser: false,
		},
	})
	if err != nil {
		log.Error("cannot send reminder reply", "err", err, "channel", m.ChannelID)
	}
}

func (c *ReminderService) react(m *discordgo.MessageCreate, emoji string) {
	if err := c.session.MessageReactionAdd(m.ChannelID, m.ID, emoji); err != nil {
		log.Debug("cannot add reminder reaction", "err", err, "channel", m.ChannelID)
	}
}

func looksLikeReminderRequest(text string) bool {
	return reminderKeywordPattern.MatchString(text)
}

func formatReminderList(items []*reminders.Reminder) string {
	if len(items) == 0 {
		return "No reminders or events."
	}

	const maxItems = 10
	limit := len(items)
	if limit > maxItems {
		limit = maxItems
	}

	var builder strings.Builder
	builder.WriteString("Your reminders and events:\n")
	for _, item := range items[:limit] {
		label := "Reminder"
		if item.Kind == reminders.KindEvent {
			label = "Event"
		}
		id := item.ID
		if len(id) > 8 {
			id = id[:8]
		}
		fmt.Fprintf(&builder, "`%s` %s · <t:%d:f> (<t:%d:R>)", id, label, item.NextRunAt.Unix(), item.NextRunAt.Unix())
		if item.Text != "" {
			fmt.Fprintf(&builder, " · %s", truncateRunes(item.Text, 80))
		}
		builder.WriteString("\n")
	}
	if len(items) > maxItems {
		fmt.Fprintf(&builder, "...and %d more. Use /reminders to manage.", len(items)-maxItems)
	}
	return strings.TrimSpace(builder.String())
}

func parseStart(value string, loc *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty start time")
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, loc); err == nil {
			return parsed.In(loc), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse start time %q", value)
}
