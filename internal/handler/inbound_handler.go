package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/service"
)

type InboundHandler struct {
	inboundService *service.InboundService
}

func NewInboundHandler(inboundService *service.InboundService) *InboundHandler {
	return &InboundHandler{inboundService: inboundService}
}

func (h *InboundHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	return r
}

type SESNotification struct {
	Type      string            `json:"type"`
	Message   string            `json:"message"`
	Timestamp string            `json:"timestamp"`
	Receipt   SESReceipt        `json:"receipt"`
	Content   string            `json:"content"`
	MessageID string            `json:"messageId"`
	Headers   map[string]string `json:"headers,omitempty"`
}

type SESReceipt struct {
	Action struct {
		Type       string `json:"type"`
		BucketName string `json:"bucketName"`
		ObjectKey  string `json:"objectKey"`
	} `json:"action"`
}

func (h *InboundHandler) HandleSESPayload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var notification SESNotification
	if err := json.Unmarshal(body, &notification); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	if notification.Type == "Notification" && notification.Message != "" {
		var msgContent SESNotification
		if err := json.Unmarshal([]byte(notification.Message), &msgContent); err != nil {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ignored"})
			return
		}
		notification = msgContent
	}

	if notification.Content == "" {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "no content"})
		return
	}

	from, to, subject := extractInboundHeaders(notification.Content)

	if from == "" || len(to) == 0 {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "invalid email"})
		return
	}

	teamID, err := h.teamForRecipients(r.Context(), to)
	if err != nil {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "domain not found"})
		return
	}

	rawS3Key := ""
	if notification.Receipt.Action.ObjectKey != "" {
		rawS3Key = notification.Receipt.Action.ObjectKey
	}

	err = h.inboundService.ProcessEmail(
		r.Context(),
		teamID,
		from,
		to,
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
	json.NewEncoder(w).Encode(map[string]string{"status": "processed"})
}

func (h *InboundHandler) teamForRecipients(ctx context.Context, recipients []string) (uuid.UUID, error) {
	var teamID uuid.UUID
	for _, recipient := range recipients {
		domainPart := inboundRecipientDomain(recipient)
		if domainPart == "" {
			return uuid.Nil, fmt.Errorf("invalid recipient domain")
		}
		candidate, err := h.inboundService.GetTeamByDomain(ctx, domainPart)
		if err != nil {
			return uuid.Nil, err
		}
		if teamID == uuid.Nil {
			teamID = candidate
			continue
		}
		if candidate != teamID {
			return uuid.Nil, fmt.Errorf("recipients belong to different teams")
		}
	}
	if teamID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("recipient team not found")
	}
	return teamID, nil
}

func inboundRecipientDomain(address string) string {
	parsed, err := mail.ParseAddress(address)
	if err != nil {
		return ""
	}
	separator := strings.LastIndex(parsed.Address, "@")
	if separator < 0 || separator == len(parsed.Address)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(parsed.Address[separator+1:], "."))
}

func (h *InboundHandler) List(w http.ResponseWriter, r *http.Request) {
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
	message, err := mail.ReadMessage(strings.NewReader(content))
	if err != nil {
		return "", nil, ""
	}

	fromAddress, err := mail.ParseAddress(message.Header.Get("From"))
	if err != nil {
		return "", nil, ""
	}
	recipients, err := mail.ParseAddressList(message.Header.Get("To"))
	if err != nil {
		return "", nil, ""
	}
	to := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		to = append(to, recipient.Address)
	}
	return fromAddress.Address, to, message.Header.Get("Subject")
}
