package notification

import "time"

type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelPush  Channel = "push"
	ChannelBoth  Channel = "both"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusSent    Status = "sent"
	StatusFailed  Status = "failed"
)

type Notification struct {
	ID              string
	Type            string
	RecipientUserID string
	RecipientEmail  string
	Subject         string
	Body            string
	Channel         Channel
	Status          Status
	CreatedAt       time.Time
}

func NewNotification(notificationType, recipientUserID, recipientEmail, subject, body string, channel Channel) Notification {
	return Notification{
		Type:            notificationType,
		RecipientUserID: recipientUserID,
		RecipientEmail:  recipientEmail,
		Subject:         subject,
		Body:            body,
		Channel:         channel,
		Status:          StatusPending,
		CreatedAt:       time.Now().UTC(),
	}
}

type DeviceToken struct {
	UserID       string
	Token        string
	Platform     string
	RegisteredAt time.Time
}

func NewDeviceToken(userID, token, platform string) DeviceToken {
	return DeviceToken{
		UserID:       userID,
		Token:        token,
		Platform:     platform,
		RegisteredAt: time.Now().UTC(),
	}
}
