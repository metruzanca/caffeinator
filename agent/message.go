package agent

import (
	"strings"
	"time"
)

// User is a person or bot in a conversation.
type User struct {
	ID   string
	Name string
	Bot  bool
}

// Attachment describes a file attached to a message.
type Attachment struct {
	Filename  string
	MediaType string
	URL       string
	Size      int64
}

// Turn is one message in a conversation.
type Turn struct {
	ID          string
	Author      User
	Content     string
	Timestamp   time.Time
	Attachments []Attachment
}

// Image references an image the model can see.
type Image struct {
	Label  string
	URL    string
	Detail string
}

func attachmentSummary(attachments []Attachment) string {
	if len(attachments) == 0 {
		return ""
	}

	names := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		name := strings.TrimSpace(attachment.Filename)
		if name == "" {
			name = "attachment"
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}
	return "[attachments: " + strings.Join(names, ", ") + "]"
}
