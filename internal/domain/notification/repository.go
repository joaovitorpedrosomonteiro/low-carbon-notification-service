package notification

import "context"

type Repository interface {
	Save(ctx context.Context, notification Notification) error
	FindByID(ctx context.Context, id string) (Notification, error)
	FindByRecipientUserID(ctx context.Context, userID string, limit int) ([]Notification, error)
	UpdateStatus(ctx context.Context, id string, status Status) error
}

type DeviceTokenRepository interface {
	Save(ctx context.Context, token DeviceToken) error
	Delete(ctx context.Context, userID, token string) error
	FindByUserID(ctx context.Context, userID string) ([]DeviceToken, error)
}
