package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/joaovitorpedrosomonteiro/low-carbon-notification-service/internal/domain/notification"
)

type NotificationRepository struct {
	pool *pgxpool.Pool
}

func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

func (r *NotificationRepository) Save(ctx context.Context, n notification.Notification) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO notifications (id, type, recipient_user_id, recipient_email, subject, body, channel, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		n.ID, n.Type, n.RecipientUserID, n.RecipientEmail,
		n.Subject, n.Body, string(n.Channel), string(n.Status), n.CreatedAt,
	)
	return err
}

func (r *NotificationRepository) FindByID(ctx context.Context, id string) (notification.Notification, error) {
	var n notification.Notification
	var channel, status string

	err := r.pool.QueryRow(ctx,
		`SELECT id, type, recipient_user_id, recipient_email, subject, body, channel, status, created_at
		 FROM notifications WHERE id = $1`, id,
	).Scan(&n.ID, &n.Type, &n.RecipientUserID, &n.RecipientEmail,
		&n.Subject, &n.Body, &channel, &status, &n.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notification.Notification{}, errors.New("notification not found")
		}
		return notification.Notification{}, err
	}

	n.Channel = notification.Channel(channel)
	n.Status = notification.Status(status)
	return n, nil
}

func (r *NotificationRepository) FindByRecipientUserID(ctx context.Context, userID string, limit int) ([]notification.Notification, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, type, recipient_user_id, recipient_email, subject, body, channel, status, created_at
		 FROM notifications WHERE recipient_user_id = $1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []notification.Notification
	for rows.Next() {
		var n notification.Notification
		var channel, status string
		if err := rows.Scan(&n.ID, &n.Type, &n.RecipientUserID, &n.RecipientEmail,
			&n.Subject, &n.Body, &channel, &status, &n.CreatedAt); err != nil {
			return nil, err
		}
		n.Channel = notification.Channel(channel)
		n.Status = notification.Status(status)
		notifications = append(notifications, n)
	}
	return notifications, nil
}

func (r *NotificationRepository) UpdateStatus(ctx context.Context, id string, status notification.Status) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET status = $1 WHERE id = $2`, string(status), id)
	return err
}

type DeviceTokenRepository struct {
	pool *pgxpool.Pool
}

func NewDeviceTokenRepository(pool *pgxpool.Pool) *DeviceTokenRepository {
	return &DeviceTokenRepository{pool: pool}
}

func (r *DeviceTokenRepository) Save(ctx context.Context, token notification.DeviceToken) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO device_tokens (user_id, token, platform, registered_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, token) DO UPDATE SET platform = $3, registered_at = $4`,
		token.UserID, token.Token, token.Platform, token.RegisteredAt,
	)
	return err
}

func (r *DeviceTokenRepository) Delete(ctx context.Context, userID, token string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM device_tokens WHERE user_id = $1 AND token = $2`, userID, token)
	return err
}

func (r *DeviceTokenRepository) FindByUserID(ctx context.Context, userID string) ([]notification.DeviceToken, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, token, platform, registered_at
		 FROM device_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []notification.DeviceToken
	for rows.Next() {
		var t notification.DeviceToken
		if err := rows.Scan(&t.UserID, &t.Token, &t.Platform, &t.RegisteredAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

type MockNotificationRepository struct{}

func NewMockNotificationRepository() *MockNotificationRepository {
	return &MockNotificationRepository{}
}

func (m *MockNotificationRepository) Save(ctx context.Context, n notification.Notification) error {
	return nil
}

func (m *MockNotificationRepository) FindByID(ctx context.Context, id string) (notification.Notification, error) {
	return notification.Notification{}, errors.New("not found")
}

func (m *MockNotificationRepository) FindByRecipientUserID(ctx context.Context, userID string, limit int) ([]notification.Notification, error) {
	return nil, nil
}

func (m *MockNotificationRepository) UpdateStatus(ctx context.Context, id string, status notification.Status) error {
	return nil
}

type MockDeviceTokenRepository struct{}

func NewMockDeviceTokenRepository() *MockDeviceTokenRepository {
	return &MockDeviceTokenRepository{}
}

func (m *MockDeviceTokenRepository) Save(ctx context.Context, token notification.DeviceToken) error {
	return nil
}

func (m *MockDeviceTokenRepository) Delete(ctx context.Context, userID, token string) error {
	return nil
}

func (m *MockDeviceTokenRepository) FindByUserID(ctx context.Context, userID string) ([]notification.DeviceToken, error) {
	return nil, nil
}

func now() time.Time {
	return time.Now().UTC()
}
