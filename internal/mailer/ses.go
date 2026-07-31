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
	client    *sesv2.Client
	configSet string
	logger    *slog.Logger
}

func NewSESMailer(client *sesv2.Client, configSet string, logger *slog.Logger) *SESMailer {
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
		return "", domain.NewDeliveryError(fmt.Errorf("ses send failed: %w", err), sesErrorRetryable(err))
	}

	m.logger.Info("email sent via SES",
		"email_id", email.ID,
		"from", email.From,
	)

	if output.MessageId == nil || strings.TrimSpace(*output.MessageId) == "" {
		return "", domain.NewDeliveryError(fmt.Errorf("ses send returned no message id"), false)
	}
	return *output.MessageId, nil
}

func sesErrorRetryable(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := strings.ToLower(apiErr.ErrorCode())
		switch {
		case strings.Contains(code, "throttl"), strings.Contains(code, "timeout"), strings.Contains(code, "unavailable"), strings.Contains(code, "internal"):
			return true
		case strings.Contains(code, "reject"), strings.Contains(code, "invalid"), strings.Contains(code, "notverified"), strings.Contains(code, "configuration"):
			return false
		}
	}
	return true
}
