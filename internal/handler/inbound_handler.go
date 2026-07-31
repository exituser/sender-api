package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/inbound"
	"github.com/sender-api/sender-api/internal/service"
	"github.com/sender-api/sender-api/pkg/sns"
)

type InboundHandler struct {
	inboundService *service.InboundService
	inboundToken   string
	awsRegion      string
	snsTopicArn    string
}

func NewInboundHandler(inboundService *service.InboundService, inboundToken, awsRegion, snsTopicArn string) *InboundHandler {
	return &InboundHandler{inboundService: inboundService, inboundToken: inboundToken, awsRegion: awsRegion, snsTopicArn: snsTopicArn}
}

func (h *InboundHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	return r
}

func (h *InboundHandler) HandleSESPayload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	var envelope inbound.SNSNotification
	if err := json.Unmarshal(body, &envelope); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	var notification *inbound.SESNotification
	if envelope.Type == "Notification" && envelope.Message != "" {
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
		if h.snsTopicArn != "" && envelope.TopicArn != h.snsTopicArn {
			writeError(w, "invalid SNS notification", http.StatusUnauthorized)
			return
		}
		msgContent, err := inbound.DecodeNotification([]byte(envelope.Message))
		if err != nil {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ignored"})
			return
		}
		notification = msgContent
		if notification.MessageID == "" {
			notification.MessageID = envelope.MessageID
		}
	} else {
		providedToken := r.Header.Get("X-Inbound-Token")
		if h.inboundToken == "" {
			writeError(w, "inbound webhook token is not configured", http.StatusServiceUnavailable)
			return
		}
		if subtle.ConstantTimeCompare([]byte(providedToken), []byte(h.inboundToken)) != 1 {
			writeError(w, "invalid inbound webhook token", http.StatusUnauthorized)
			return
		}
		notification, err = inbound.DecodeNotification(body)
		if err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
	}

	if notification == nil || notification.Content == "" {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "no content"})
		return
	}

	from, to, subject := extractInboundHeaders(notification.Content)

	if from == "" || len(to) == 0 {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "invalid email"})
		return
	}

	routingRecipients := to
	if len(notification.Receipt.Recipients) > 0 {
		routingRecipients = notification.Receipt.Recipients
	}
	teamID, err := h.teamForRecipients(r.Context(), routingRecipients)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "domain not found"})
		return
	}

	rawS3Key := ""
	if notification.Receipt.Action.ObjectKey != "" {
		rawS3Key = notification.Receipt.Action.ObjectKey
	}

	var messageID *string
	if strings.TrimSpace(notification.MessageID) != "" {
		value := strings.TrimSpace(notification.MessageID)
		messageID = &value
	}
	err = h.inboundService.ProcessEmailWithMessageID(
		r.Context(),
		teamID,
		messageID,
		from,
		routingRecipients,
		subject,
		notification.Content,
		"",
		nil,
		rawS3Key,
	)
	if err != nil {
		http.Error(w, `{"error":"failed to process"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "processed"})
}

func (h *InboundHandler) teamForRecipients(ctx context.Context, recipients []string) (uuid.UUID, error) {
	return h.inboundService.TeamForRecipients(ctx, recipients)
}

func inboundRecipientDomain(address string) string {
	return inbound.RecipientDomain(address)
}

func (h *InboundHandler) List(w http.ResponseWriter, r *http.Request) {
	if !requirePermission(w, r, "read") {
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}
	limit, offset := 50, 0
	if value := r.URL.Query().Get("limit"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			limit = parsed
		}
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			offset = parsed
		}
	}
	result, err := h.inboundService.List(r.Context(), teamID, limit, offset)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, result, http.StatusOK)
}

func extractInboundHeaders(content string) (string, []string, string) {
	message, err := inbound.ParseRawMessage(content)
	if err != nil {
		return "", nil, ""
	}
	return message.From, message.To, message.Subject
}
