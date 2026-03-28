package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/joaovitorpedrosomonteiro/low-carbon-notification-service/internal/domain/notification"
)

type EventEnvelope struct {
	EventID     string          `json:"event_id"`
	EventType  string          `json:"event_type"`
	OccurredAt time.Time       `json:"occurred_at"`
	SchemaVer  string          `json:"schema_version"`
	Payload     json.RawMessage `json:"payload"`
}

type InventoryStateChangedPayload struct {
	InventoryID       string   `json:"inventoryID"`
	FromState         string   `json:"fromState"`
	ToState           string   `json:"toState"`
	ActorID           string   `json:"actorID"`
	ReviewMessage     *string  `json:"reviewMessage,omitempty"`
	RecipientUserIDs  []string `json:"recipientUserIDs"`
	RecipientEmails   []string `json:"recipientEmails"`
}

type UserCreatedPayload struct {
	UserID            string  `json:"userID"`
	Role              string  `json:"role"`
	Email             string  `json:"email"`
	TemporaryPassword string  `json:"temporaryPassword"`
	CompanyID         *string `json:"companyID,omitempty"`
	BranchID          *string `json:"branchID,omitempty"`
}

type UserPasswordResetPayload struct {
	UserID            string `json:"userID"`
	Role              string `json:"role"`
	Email             string `json:"email"`
	TemporaryPassword string `json:"temporaryPassword"`
}

type AuditorAccessGrantedPayload struct {
	AuditorID    string  `json:"auditorID"`
	AuditorEmail string  `json:"auditorEmail"`
	Scope        string  `json:"scope"`
	ResourceName string  `json:"resourceName"`
	InventoryID  *string `json:"inventoryID,omitempty"`
	BranchID     *string `json:"branchID,omitempty"`
	CompanyID    *string `json:"companyID,omitempty"`
}

type DocumentReadyForSigningPayload struct {
	InventoryID       string `json:"inventoryID"`
	AuditorID         string `json:"auditorID"`
	AuditorEmail      string `json:"auditorEmail"`
	UnsignedDocumentURL string `json:"unsignedDocumentUrl"`
}

type DocumentGeneratedPayload struct {
	InventoryID       string `json:"inventoryID"`
	GCSUri            string `json:"gcsUri"`
	CompanyAdminEmail string `json:"companyAdminEmail"`
}

type DocumentGenerationFailedPayload struct {
	InventoryID string `json:"inventoryID"`
	Reason      string `json:"reason"`
}

type PasswordResetRequestedPayload struct {
	UserID    string `json:"userID"`
	Email     string `json:"email"`
	ResetLink string `json:"resetLink"`
}

type EmailSender interface {
	Send(ctx context.Context, to, subject, body string) error
}

type PushSender interface {
	Send(ctx context.Context, token, title, body string) error
}

type EventHandler struct {
	notificationRepo notification.Repository
	deviceTokenRepo  notification.DeviceTokenRepository
	emailSender      EmailSender
	pushSender       PushSender
}

func NewEventHandler(
	notificationRepo notification.Repository,
	deviceTokenRepo notification.DeviceTokenRepository,
	emailSender EmailSender,
	pushSender PushSender,
) *EventHandler {
	return &EventHandler{
		notificationRepo: notificationRepo,
		deviceTokenRepo:  deviceTokenRepo,
		emailSender:      emailSender,
		pushSender:       pushSender,
	}
}

func (h *EventHandler) HandleInventoryStateChanged(ctx context.Context, envelope EventEnvelope) error {
	var payload InventoryStateChangedPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal InventoryStateChanged payload: %w", err)
	}

	var subject, body string

	switch payload.ToState {
	case "to_provide_evidence":
		subject = "Inventário pronto para coleta de evidências"
		body = fmt.Sprintf(
			`<h2>Inventário pronto para coleta</h2>
			<p>O inventário <strong>%s</strong> está pronto para que você forneça as evidências necessárias.</p>
			<p>Por favor, acesse o sistema para fazer o upload dos documentos de evidência.</p>`,
			payload.InventoryID,
		)
	case "for_auditing":
		subject = "Inventário enviado para auditoria"
		body = fmt.Sprintf(
			`<h2>Inventário enviado para auditoria</h2>
			<p>O inventário <strong>%s</strong> foi enviado para auditoria.</p>
			<p>Por favor, acesse o sistema para revisar o inventário.</p>`,
			payload.InventoryID,
		)
	case "for_review":
		subject = "Inventário devolvido para revisão"
		body = fmt.Sprintf(
			`<h2>Inventário devolvido para revisão</h2>
			<p>O inventário <strong>%s</strong> foi devolvido para revisão.</p>`,
			payload.InventoryID,
		)
		if payload.ReviewMessage != nil {
			body += fmt.Sprintf(`<p><strong>Mensagem do auditor:</strong> %s</p>`, *payload.ReviewMessage)
		}
		body += `<p>Por favor, acesse o sistema para fazer as correções necessárias.</p>`
	case "audited":
		subject = "Inventário auditado com sucesso"
		body = fmt.Sprintf(
			`<h2>Inventário auditado</h2>
			<p>O inventário <strong>%s</strong> foi auditado com sucesso.</p>
			<p>O documento assinado estará disponível em breve.</p>`,
			payload.InventoryID,
		)
	default:
		log.Printf("[Handler] Unhandled InventoryStateChanged to state: %s", payload.ToState)
		return nil
	}

	for i, userID := range payload.RecipientUserIDs {
		email := ""
		if i < len(payload.RecipientEmails) {
			email = payload.RecipientEmails[i]
		}

		n := notification.NewNotification(
			"InventoryStateChanged",
			userID,
			email,
			subject,
			body,
			notification.ChannelBoth,
		)
		if err := h.sendNotification(ctx, n); err != nil {
			log.Printf("[Handler] Failed to send notification to user %s: %v", userID, err)
		}
	}

	return nil
}

func (h *EventHandler) HandleAuditorAccessGranted(ctx context.Context, envelope EventEnvelope) error {
	var payload AuditorAccessGrantedPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal AuditorAccessGranted payload: %w", err)
	}

	subject := "Acesso de auditoria concedido"
	body := fmt.Sprintf(
		`<h2>Acesso de auditoria concedido</h2>
		<p>Você recebeu acesso de auditoria para: <strong>%s</strong></p>
		<p>Escopo: %s</p>
		<p>Acesse o sistema para revisar os inventários disponíveis.</p>`,
		payload.ResourceName, payload.Scope,
	)

	n := notification.NewNotification(
		"AuditorAccessGranted",
		payload.AuditorID,
		payload.AuditorEmail,
		subject,
		body,
		notification.ChannelBoth,
	)
	return h.sendNotification(ctx, n)
}

func (h *EventHandler) HandleDocumentReadyForSigning(ctx context.Context, envelope EventEnvelope) error {
	var payload DocumentReadyForSigningPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal DocumentReadyForSigning payload: %w", err)
	}

	subject := "Documento pronto para assinatura"
	body := fmt.Sprintf(
		`<h2>Documento pronto para assinatura</h2>
		<p>O documento do inventário <strong>%s</strong> está pronto para assinatura.</p>
		<p>Por favor, acesse o sistema para baixar, assinar e fazer o upload do documento.</p>
		<p><a href="%s">Acessar documento</a></p>`,
		payload.InventoryID, payload.UnsignedDocumentURL,
	)

	n := notification.NewNotification(
		"DocumentReadyForSigning",
		payload.AuditorID,
		payload.AuditorEmail,
		subject,
		body,
		notification.ChannelBoth,
	)
	return h.sendNotification(ctx, n)
}

func (h *EventHandler) HandleDocumentGenerated(ctx context.Context, envelope EventEnvelope) error {
	var payload DocumentGeneratedPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal DocumentGenerated payload: %w", err)
	}

	subject := "Documento gerado com sucesso"
	body := fmt.Sprintf(
		`<h2>Documento gerado</h2>
		<p>O documento do inventário <strong>%s</strong> foi gerado e assinado com sucesso.</p>
		<p>Acesse o sistema para visualizar e baixar o documento.</p>`,
		payload.InventoryID,
	)

	n := notification.NewNotification(
		"DocumentGenerated",
		"",
		payload.CompanyAdminEmail,
		subject,
		body,
		notification.ChannelEmail,
	)
	return h.sendNotification(ctx, n)
}

func (h *EventHandler) HandleDocumentGenerationFailed(ctx context.Context, envelope EventEnvelope) error {
	var payload DocumentGenerationFailedPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal DocumentGenerationFailed payload: %w", err)
	}

	log.Printf("[Handler] Document generation failed for inventory %s: %s", payload.InventoryID, payload.Reason)
	return nil
}

func (h *EventHandler) HandleUserCreated(ctx context.Context, envelope EventEnvelope) error {
	var payload UserCreatedPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal UserCreated payload: %w", err)
	}

	subject := "Bem-vindo ao Low Carbon"
	body := fmt.Sprintf(
		`<h2>Bem-vindo ao Low Carbon!</h2>
		<p>Sua conta foi criada com sucesso.</p>
		<p><strong>Email:</strong> %s</p>
		<p><strong>Senha temporária:</strong> %s</p>
		<p>Por favor, faça login e altere sua senha no primeiro acesso.</p>`,
		payload.Email, payload.TemporaryPassword,
	)

	n := notification.NewNotification(
		"UserCreated",
		payload.UserID,
		payload.Email,
		subject,
		body,
		notification.ChannelEmail,
	)
	return h.sendNotification(ctx, n)
}

func (h *EventHandler) HandleUserPasswordReset(ctx context.Context, envelope EventEnvelope) error {
	var payload UserPasswordResetPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal UserPasswordReset payload: %w", err)
	}

	subject := "Senha redefinida"
	body := fmt.Sprintf(
		`<h2>Senha redefinida</h2>
		<p>Sua senha foi redefinida por um administrador.</p>
		<p><strong>Nova senha temporária:</strong> %s</p>
		<p>Por favor, faça login e altere sua senha.</p>`,
		payload.TemporaryPassword,
	)

	n := notification.NewNotification(
		"UserPasswordReset",
		payload.UserID,
		payload.Email,
		subject,
		body,
		notification.ChannelEmail,
	)
	return h.sendNotification(ctx, n)
}

func (h *EventHandler) HandlePasswordResetRequested(ctx context.Context, envelope EventEnvelope) error {
	var payload PasswordResetRequestedPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal PasswordResetRequested payload: %w", err)
	}

	subject := "Redefinição de senha"
	body := fmt.Sprintf(
		`<h2>Redefinição de senha</h2>
		<p>Você solicitou a redefinição de sua senha.</p>
		<p>Clique no link abaixo para redefinir sua senha:</p>
		<p><a href="%s">Redefinir senha</a></p>
		<p>Este link é válido por 1 hora.</p>
		<p>Se você não solicitou esta redefinição, ignore este email.</p>`,
		payload.ResetLink,
	)

	n := notification.NewNotification(
		"PasswordResetRequested",
		payload.UserID,
		payload.Email,
		subject,
		body,
		notification.ChannelEmail,
	)
	return h.sendNotification(ctx, n)
}

func (h *EventHandler) sendNotification(ctx context.Context, n notification.Notification) error {
	n.ID = uuid.New().String()

	if err := h.notificationRepo.Save(ctx, n); err != nil {
		log.Printf("[Handler] Failed to save notification: %v", err)
	}

	switch n.Channel {
	case notification.ChannelEmail, notification.ChannelBoth:
		if n.RecipientEmail != "" {
			if err := h.emailSender.Send(ctx, n.RecipientEmail, n.Subject, n.Body); err != nil {
				log.Printf("[Handler] Email send failed: %v", err)
				return h.notificationRepo.UpdateStatus(ctx, n.ID, notification.StatusFailed)
			}
		}
	}

	if n.Channel == notification.ChannelPush || n.Channel == notification.ChannelBoth {
		if n.RecipientUserID != "" {
			tokens, err := h.deviceTokenRepo.FindByUserID(ctx, n.RecipientUserID)
			if err != nil {
				log.Printf("[Handler] Failed to find device tokens: %v", err)
			} else {
				for _, dt := range tokens {
					if err := h.pushSender.Send(ctx, dt.Token, n.Subject, stripHTML(n.Body)); err != nil {
						log.Printf("[Handler] Push send failed for token: %v", err)
					}
				}
			}
		}
	}

	return h.notificationRepo.UpdateStatus(ctx, n.ID, notification.StatusSent)
}

func stripHTML(s string) string {
	result := make([]byte, 0, len(s))
	inTag := false
	for _, c := range s {
		if c == '<' {
			inTag = true
			continue
		}
		if c == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result = append(result, byte(c))
		}
	}
	return string(result)
}
