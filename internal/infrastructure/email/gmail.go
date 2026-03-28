package email

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
	gmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type GmailSender struct {
	service *gmail.Service
	sender  string
}

func NewGmailSender(ctx context.Context) (*GmailSender, error) {
	sender := os.Getenv("GMAIL_SENDER")
	if sender == "" {
		sender = "noreply@lowcarbon.com"
	}

	credsJSON := os.Getenv("GMAIL_SERVICE_ACCOUNT_JSON")
	if credsJSON == "" {
		return nil, fmt.Errorf("GMAIL_SERVICE_ACCOUNT_JSON not set")
	}

	config, err := google.JWTConfigFromJSON([]byte(credsJSON), gmail.GmailSendScope)
	if err != nil {
		return nil, fmt.Errorf("failed to parse service account: %w", err)
	}

	config.Subject = sender

	srv, err := gmail.NewService(ctx, option.WithHTTPClient(config.Client(ctx)))
	if err != nil {
		return nil, fmt.Errorf("failed to create gmail service: %w", err)
	}

	return &GmailSender{service: srv, sender: sender}, nil
}

func (g *GmailSender) Send(ctx context.Context, to, subject, body string) error {
	from := g.sender

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	raw := base64.URLEncoding.EncodeToString([]byte(msg.String()))

	gmsg := &gmail.Message{Raw: raw}
	_, err := g.service.Users.Messages.Send("me", gmsg).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("gmail send: %w", err)
	}

	log.Printf("[Gmail] Sent email to %s: %s", to, subject)
	return nil
}

type MockEmailSender struct{}

func NewMockEmailSender() *MockEmailSender {
	return &MockEmailSender{}
}

func (m *MockEmailSender) Send(ctx context.Context, to, subject, body string) error {
	log.Printf("[MockEmail] To: %s | Subject: %s | Body: %s", to, subject, truncate(body, 200))
	return nil
}

type SMTPEmailSender struct {
	host     string
	port     string
	username string
	password string
	from     string
}

func NewSMTPEmailSender() *SMTPEmailSender {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("SMTP_PORT")
	if port == "" {
		port = "1025"
	}

	return &SMTPEmailSender{
		host:     host,
		port:     port,
		username: os.Getenv("SMTP_USERNAME"),
		password: os.Getenv("SMTP_PASSWORD"),
		from:     os.Getenv("SMTP_FROM"),
	}
}

func (s *SMTPEmailSender) Send(ctx context.Context, to, subject, body string) error {
	from := s.from
	if from == "" {
		from = "noreply@lowcarbon.local"
	}

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	addr := s.host + ":" + s.port
	err := smtp.SendMail(addr, auth, from, []string{to}, []byte(msg.String()))
	if err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}

	log.Printf("[SMTP] Sent email to %s: %s", to, subject)
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
