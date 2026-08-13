package mailer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/smithy-go"
	"github.com/sender-api/sender-api/internal/domain"
)

type SESMailer struct {
	client    SESClient
	configSet string
	logger    *slog.Logger
}

type SESClient interface {
	SendEmail(context.Context, *sesv2.SendEmailInput, ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
}

func NewSESMailer(client SESClient, configSet string, logger *slog.Logger) *SESMailer {
	return &SESMailer{
		client:    client,
		configSet: configSet,
		logger:    logger,
	}
}

func (m *SESMailer) Send(ctx context.Context, email *domain.Email) (string, error) {
	input := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(email.From),
		Destination: &types.Destination{
			ToAddresses: email.To,
		},
		ReplyToAddresses: email.ReplyTo,
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data: aws.String(email.Subject),
				},
				Body: &types.Body{},
			},
		},
	}
	input.EmailTags = append(input.EmailTags, types.MessageTag{
		Name: aws.String("sender_email_id"), Value: aws.String(email.ID.String()),
	})
	if email.SendAttemptID != nil {
		input.EmailTags = append(input.EmailTags, types.MessageTag{
			Name: aws.String("sender_attempt_id"), Value: aws.String(email.SendAttemptID.String()),
		})
	}

	if len(email.CC) > 0 {
		input.Destination.CcAddresses = email.CC
	}
	if len(email.BCC) > 0 {
		input.Destination.BccAddresses = email.BCC
	}

	if email.HTML != "" {
		input.Content.Simple.Body.Html = &types.Content{
			Data: aws.String(email.HTML),
		}
	}
	if email.Text != "" {
		input.Content.Simple.Body.Text = &types.Content{
			Data: aws.String(email.Text),
		}
	}

	if len(email.Attachments) > 0 {
		input.Content.Simple.Attachments = make([]types.Attachment, 0, len(email.Attachments))
		for _, attachment := range email.Attachments {
			contentType := mime.TypeByExtension(filepath.Ext(attachment.Filename))
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			input.Content.Simple.Attachments = append(input.Content.Simple.Attachments, types.Attachment{
				FileName:                aws.String(attachment.Filename),
				RawContent:              attachment.Content,
				ContentType:             aws.String(contentType),
				ContentDisposition:      types.AttachmentContentDispositionAttachment,
				ContentTransferEncoding: types.AttachmentContentTransferEncodingBase64,
			})
		}
	}

	if m.configSet != "" {
		input.ConfigurationSetName = aws.String(m.configSet)
	}

	if len(email.Headers) > 0 {
		input.Content.Simple.Headers = make([]types.MessageHeader, 0, len(email.Headers))
		for key, value := range email.Headers {
			input.Content.Simple.Headers = append(input.Content.Simple.Headers, types.MessageHeader{
				Name:  aws.String(key),
				Value: aws.String(value),
			})
		}
	}

	output, err := m.client.SendEmail(ctx, input)
	if err != nil {
		m.logger.Error("failed to send email via SES",
			"email_id", email.ID,
			"error", err,
		)
		retryable, smtpCode, enhancedCode, providerCode := sesErrorDetails(err)
		return "", domain.NewDeliveryErrorWithOutcomeDetails(
			fmt.Errorf("ses send failed: %w", err), retryable, sesOutcomeUnknown(err), smtpCode, enhancedCode, providerCode,
		)
	}

	m.logger.Info("email sent via SES",
		"email_id", email.ID,
	)

	if output.MessageId == nil || strings.TrimSpace(*output.MessageId) == "" {
		return "", domain.NewDeliveryErrorWithOutcomeDetails(
			fmt.Errorf("ses send returned no message id"), false, true, 0, "", "missing_message_id",
		)
	}
	return *output.MessageId, nil
}

func sesOutcomeUnknown(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return true
	}
	if apiErr.ErrorFault() == smithy.FaultServer {
		return true
	}
	code := strings.ToLower(strings.TrimSpace(apiErr.ErrorCode()))
	for _, marker := range []string{
		"messagerejected", "reject", "invalid", "validation", "badrequest",
		"notverified", "configuration", "accessdenied", "sendingpaused",
		"notfound", "account.suspended",
	} {
		if strings.Contains(code, marker) {
			return false
		}
	}
	// A provider error without a clearly definitive client-side rejection can
	// arrive after the request body was accepted. Quarantine it instead of
	// risking a duplicate submission.
	return true
}

// sesErrorDetails maps provider failures into the RFC 3463-style status model
// exposed by the API. SES returns AWS error codes rather than SMTP responses,
// so these codes are normalized equivalents, not wire-level SMTP evidence.
func sesErrorDetails(err error) (retryable bool, smtpCode int, enhancedCode, providerCode string) {
	if errors.Is(err, context.Canceled) {
		return false, 0, "", "context_canceled"
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		providerCode = apiErr.ErrorCode()
		code := strings.ToLower(providerCode)
		switch {
		case strings.Contains(code, "throttl"), strings.Contains(code, "timeout"), strings.Contains(code, "unavailable"), strings.Contains(code, "internal"):
			return true, 451, "4.3.0", providerCode
		case strings.Contains(code, "too-many-requests"), strings.Contains(code, "toomanyrequests"):
			return true, 421, "4.7.0", providerCode
		case strings.Contains(code, "reject"), strings.Contains(code, "invalid"), strings.Contains(code, "notverified"), strings.Contains(code, "configuration"), strings.Contains(code, "accessdenied"), strings.Contains(code, "sendingpaused"):
			return false, 554, "5.7.1", providerCode
		}
	}
	return true, 451, "4.3.0", providerCode
}
