package inbound

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sender-api/sender-api/pkg/sns"
)

type SNSNotification struct {
	Type             string `json:"Type"`
	Message          string `json:"Message"`
	MessageID        string `json:"MessageId"`
	Subject          string `json:"Subject"`
	Timestamp        string `json:"Timestamp"`
	TopicArn         string `json:"TopicArn"`
	SigningCertURL   string `json:"SigningCertURL"`
	Signature        string `json:"Signature"`
	SignatureVersion string `json:"SignatureVersion"`
}

type SESNotification struct {
	Type             string     `json:"type"`
	NotificationType string     `json:"notificationType"`
	Timestamp        string     `json:"timestamp"`
	Receipt          SESReceipt `json:"receipt"`
	Content          string     `json:"content"`
	MessageID        string     `json:"messageId"`
	Mail             struct {
		MessageID string `json:"messageId"`
	} `json:"mail"`
	// SES inbound notifications have used both object and array header shapes.
	// The worker parses the raw message from S3, so preserve this optional field
	// without coupling notification decoding to either wire representation.
	Headers json.RawMessage `json:"headers,omitempty"`
}

type SESReceipt struct {
	Recipients []string `json:"recipients"`
	Action     struct {
		Type       string `json:"type"`
		BucketName string `json:"bucketName"`
		ObjectKey  string `json:"objectKey"`
	} `json:"action"`
}

func DecodeNotification(body []byte) (*SESNotification, error) {
	var notification SESNotification
	if err := json.Unmarshal(body, &notification); err != nil {
		return nil, fmt.Errorf("decode notification: %w", err)
	}
	if notification.Type == "" {
		notification.Type = notification.NotificationType
	}
	if notification.MessageID == "" {
		notification.MessageID = notification.Mail.MessageID
	}
	return &notification, nil
}

func DecodeAndVerifySNS(ctx context.Context, body []byte, region, expectedTopicArn string) (*SESNotification, error) {
	var notification SNSNotification
	if err := json.Unmarshal(body, &notification); err != nil {
		return nil, fmt.Errorf("decode SNS notification: %w", err)
	}
	if notification.Type != "Notification" || notification.Message == "" {
		return nil, fmt.Errorf("not an SNS notification")
	}
	if err := sns.VerifyNotification(ctx, sns.Notification{
		Type:             notification.Type,
		Message:          notification.Message,
		MessageID:        notification.MessageID,
		Subject:          notification.Subject,
		Timestamp:        notification.Timestamp,
		TopicArn:         notification.TopicArn,
		SigningCertURL:   notification.SigningCertURL,
		Signature:        notification.Signature,
		SignatureVersion: notification.SignatureVersion,
	}, region, time.Now().UTC()); err != nil {
		return nil, err
	}
	if expectedTopicArn != "" && notification.TopicArn != expectedTopicArn {
		return nil, fmt.Errorf("unexpected SNS topic")
	}
	inner, err := DecodeNotification([]byte(notification.Message))
	if err != nil {
		return nil, err
	}
	if inner.MessageID == "" {
		inner.MessageID = notification.MessageID
	}
	return inner, nil
}
