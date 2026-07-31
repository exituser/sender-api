package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/inbound"
	"github.com/sender-api/sender-api/internal/service"
	"github.com/sender-api/sender-api/pkg/sns"
)

type SESEventHandler struct {
	emailService *service.EmailService
	awsRegion    string
	topicArn     string
}

type sesEventPayload struct {
	EventType        string `json:"eventType"`
	NotificationType string `json:"notificationType"`
	Mail             struct {
		MessageID string `json:"messageId"`
	} `json:"mail"`
}

func NewSESEventHandler(emailService *service.EmailService, awsRegion, topicArn string) *SESEventHandler {
	return &SESEventHandler{emailService: emailService, awsRegion: awsRegion, topicArn: topicArn}
}

func (h *SESEventHandler) Handle(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, "invalid SNS body", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	var envelope inbound.SNSNotification
	if err := json.Unmarshal(body, &envelope); err != nil {
		writeError(w, "invalid SNS body", http.StatusBadRequest)
		return
	}
	if err := sns.VerifyNotification(r.Context(), sns.Notification{
		Type:             envelope.Type,
		Message:          envelope.Message,
		MessageID:        envelope.MessageID,
		Subject:          envelope.Subject,
		Timestamp:        envelope.Timestamp,
		TopicArn:         envelope.TopicArn,
		SigningCertURL:   envelope.SigningCertURL,
		Signature:        envelope.Signature,
		SignatureVersion: envelope.SignatureVersion,
	}, h.awsRegion, time.Now().UTC()); err != nil {
		writeError(w, "invalid SNS notification", http.StatusUnauthorized)
		return
	}
	if h.topicArn != "" && envelope.TopicArn != h.topicArn {
		writeError(w, "invalid SNS notification", http.StatusUnauthorized)
		return
	}

	var event sesEventPayload
	if err := json.Unmarshal([]byte(envelope.Message), &event); err != nil {
		writeJSON(w, map[string]string{"status": "ignored"}, http.StatusOK)
		return
	}
	eventType := event.EventType
	if eventType == "" {
		eventType = event.NotificationType
	}
	if eventType == "" || event.Mail.MessageID == "" {
		writeJSON(w, map[string]string{"status": "ignored"}, http.StatusOK)
		return
	}

	eventID := uuid.NewSHA1(uuid.Nil, []byte(envelope.MessageID))
	if err := h.emailService.ProcessProviderEvent(r.Context(), event.Mail.MessageID, eventType, json.RawMessage(envelope.Message), eventID); err != nil {
		if serviceErrIsProviderNotFound(err) {
			// A provider callback can legitimately arrive for an email that was
			// deleted or created before this API knew its provider ID. Retrying
			// that message forever only poisons the SNS subscription; the event
			// is still acknowledged and the correlation miss is observable in
			// application logs.
			writeJSON(w, map[string]string{"status": "ignored"}, http.StatusOK)
			return
		}
		writeError(w, "failed to process provider event", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "processed"}, http.StatusOK)
}

func serviceErrIsProviderNotFound(err error) bool {
	return errors.Is(err, service.ErrProviderEmailNotFound)
}
