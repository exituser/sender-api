package worker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/sender-api/sender-api/internal/inbound"
	"github.com/sender-api/sender-api/internal/service"
)

const maxInboundMessageSize = 10 << 20

type InboundWorker struct {
	s3Client       *s3.Client
	sqsClient      *sqs.Client
	queueURL       string
	bucket         string
	awsRegion      string
	snsTopicArn    string
	inboundService *service.InboundService
	logger         *slog.Logger
}

func NewInboundWorker(s3Client *s3.Client, sqsClient *sqs.Client, queueURL, bucket, awsRegion, snsTopicArn string, inboundService *service.InboundService, logger *slog.Logger) *InboundWorker {
	return &InboundWorker{
		s3Client:       s3Client,
		sqsClient:      sqsClient,
		queueURL:       queueURL,
		bucket:         bucket,
		awsRegion:      awsRegion,
		snsTopicArn:    snsTopicArn,
		inboundService: inboundService,
		logger:         logger,
	}
}

func (w *InboundWorker) Start(ctx context.Context) {
	w.logger.Info("inbound worker started", "queue_url_configured", w.queueURL != "")
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("inbound worker stopped")
			return
		default:
		}

		messages, err := w.sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(w.queueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20,
			VisibilityTimeout:   60,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			w.logger.Error("failed to receive inbound messages", "error", err)
			sleepContext(ctx, time.Second)
			continue
		}

		for _, message := range messages.Messages {
			if err := w.processMessage(ctx, message); err != nil {
				w.logger.Error("failed to process inbound message", "error", err)
				continue
			}
			if message.ReceiptHandle == nil {
				continue
			}
			if _, err := w.sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(w.queueURL),
				ReceiptHandle: message.ReceiptHandle,
			}); err != nil {
				w.logger.Error("failed to delete inbound SQS message", "error", err)
			}
		}
	}
}

func (w *InboundWorker) processMessage(ctx context.Context, message types.Message) error {
	if message.Body == nil {
		return fmt.Errorf("inbound SQS message body is empty")
	}
	notification, err := inbound.DecodeAndVerifySNS(ctx, []byte(*message.Body), w.awsRegion, w.snsTopicArn)
	if err != nil {
		return fmt.Errorf("verify SNS message: %w", err)
	}
	content := notification.Content
	if content == "" {
		if notification.Receipt.Action.ObjectKey == "" {
			return fmt.Errorf("inbound notification has no raw content or S3 object key")
		}
		bucket := notification.Receipt.Action.BucketName
		if bucket == "" {
			bucket = w.bucket
		}
		if bucket != w.bucket {
			return fmt.Errorf("inbound S3 bucket does not match configured bucket")
		}
		content, err = w.readRawEmail(ctx, bucket, notification.Receipt.Action.ObjectKey)
		if err != nil {
			return err
		}
	}
	parsed, err := inbound.ParseRawMessage(content)
	if err != nil {
		return err
	}
	routingRecipients := parsed.To
	if len(notification.Receipt.Recipients) > 0 {
		routingRecipients = notification.Receipt.Recipients
	}
	teamID, err := w.inboundService.TeamForRecipients(ctx, routingRecipients)
	if err != nil {
		return fmt.Errorf("resolve inbound team: %w", err)
	}
	var messageID *string
	if notification.MessageID != "" {
		value := notification.MessageID
		messageID = &value
	}
	rawS3Key := notification.Receipt.Action.ObjectKey
	return w.inboundService.ProcessEmailWithMessageID(ctx, teamID, messageID, parsed.From, routingRecipients, parsed.Subject, content, "", nil, rawS3Key)
}

func (w *InboundWorker) readRawEmail(ctx context.Context, bucket, key string) (string, error) {
	object, err := w.s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return "", fmt.Errorf("read inbound S3 object: %w", err)
	}
	defer object.Body.Close()
	content, err := io.ReadAll(io.LimitReader(object.Body, maxInboundMessageSize+1))
	if err != nil {
		return "", fmt.Errorf("read inbound S3 content: %w", err)
	}
	if len(content) > maxInboundMessageSize {
		return "", fmt.Errorf("inbound S3 object is too large")
	}
	return string(content), nil
}
