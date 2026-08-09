package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/sender-api/sender-api/internal/inbound"
	"github.com/sender-api/sender-api/internal/service"
	"github.com/sender-api/sender-api/pkg/metrics"
	"github.com/sender-api/sender-api/pkg/sns"
)

const maxInboundMessageSize = 10 << 20

var ErrDiscardInboundMessage = errors.New("discard inbound message")

type InboundWorker struct {
	s3Client          *s3.Client
	sqsClient         SQSAPI
	queueURL          string
	bucket            string
	awsRegion         string
	visibilityTimeout int32
	snsTopicArn       string
	inboundService    *service.InboundService
	logger            *slog.Logger
	concurrency       int
	metrics           *metrics.WorkerMetrics
	health            *HealthState
}

type SQSAPI interface {
	ReceiveMessage(context.Context, *sqs.ReceiveMessageInput, ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(context.Context, *sqs.DeleteMessageInput, ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
	ChangeMessageVisibility(context.Context, *sqs.ChangeMessageVisibilityInput, ...func(*sqs.Options)) (*sqs.ChangeMessageVisibilityOutput, error)
}
type InboundWorkerOption func(*InboundWorker)

func WithInboundConcurrency(value int) InboundWorkerOption {
	return func(w *InboundWorker) {
		if value > 0 {
			if value > 10 {
				value = 10
			}
			w.concurrency = value
		}
	}
}
func WithInboundMetrics(value *metrics.WorkerMetrics) InboundWorkerOption {
	return func(w *InboundWorker) {
		if value != nil {
			w.metrics = value
		}
	}
}
func WithInboundHealth(value *HealthState) InboundWorkerOption {
	return func(w *InboundWorker) {
		if value != nil {
			w.health = value
		}
	}
}

func NewInboundWorker(s3Client *s3.Client, sqsClient SQSAPI, queueURL, bucket, awsRegion, snsTopicArn string, visibilityTimeout int32, inboundService *service.InboundService, logger *slog.Logger, options ...InboundWorkerOption) *InboundWorker {
	if logger == nil {
		logger = slog.Default()
	}
	if visibilityTimeout <= 0 {
		visibilityTimeout = 120
	}
	w := &InboundWorker{
		s3Client:          s3Client,
		sqsClient:         sqsClient,
		queueURL:          queueURL,
		bucket:            bucket,
		awsRegion:         awsRegion,
		visibilityTimeout: visibilityTimeout,
		snsTopicArn:       snsTopicArn,
		inboundService:    inboundService,
		logger:            logger,
		concurrency:       4,
		metrics:           metrics.NewWorkerMetrics("inbound"),
		health:            NewHealthState(),
	}
	for _, option := range options {
		option(w)
	}
	return w
}

func (w *InboundWorker) Health() *HealthState { return w.health }

func (w *InboundWorker) Start(ctx context.Context) {
	w.logger.Info("inbound worker started", "queue_url_configured", w.queueURL != "")
	defer w.health.SetReady(false)
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("inbound worker stopped")
			return
		default:
		}

		messages, err := w.sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(w.queueURL),
			MaxNumberOfMessages: int32(w.concurrency),
			WaitTimeSeconds:     20,
			VisibilityTimeout:   w.visibilityTimeout,
		})
		if err != nil {
			w.health.SetReady(false)
			if ctx.Err() != nil {
				return
			}
			w.logger.Error("failed to receive inbound messages", "error", err)
			sleepContext(ctx, time.Second)
			continue
		}
		w.health.SetReady(true)

		var batch sync.WaitGroup
		for _, message := range messages.Messages {
			batch.Add(1)
			go func(message types.Message) {
				defer batch.Done()
				started := time.Now()
				w.metrics.Start()
				defer func() { w.metrics.ObserveAge(time.Since(started)) }()
				if err := w.processMessageWithHeartbeat(ctx, message); err != nil {
					w.metrics.Fail()
					w.logger.Error("failed to process inbound message", "error", err)
					if errors.Is(err, ErrDiscardInboundMessage) {
						if deleteErr := w.deleteMessage(ctx, message); deleteErr != nil {
							w.health.SetReady(false)
							w.logger.Error("failed to acknowledge discarded inbound message", "error", deleteErr)
						}
					}
					return
				}
				w.metrics.Complete()
				if err := w.deleteMessage(ctx, message); err != nil {
					w.health.SetReady(false)
					w.logger.Error("failed to delete inbound SQS message", "error", err)
				}
			}(message)
		}
		batch.Wait()
		w.health.Heartbeat()
	}
}

func (w *InboundWorker) processMessageWithHeartbeat(ctx context.Context, message types.Message) error {
	interval := time.Duration(w.visibilityTimeout) * time.Second / 3
	if interval < time.Second {
		interval = time.Second
	}
	heartbeatCtx, cancel := context.WithCancel(ctx)
	stop := make(chan struct{})
	done := make(chan struct{})
	defer func() {
		close(stop)
		cancel()
		<-done
	}()
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				w.health.Heartbeat()
				if message.ReceiptHandle == nil {
					continue
				}
				extendCtx, cancelExtend := context.WithTimeout(heartbeatCtx, interval)
				_, err := w.sqsClient.ChangeMessageVisibility(extendCtx, &sqs.ChangeMessageVisibilityInput{QueueUrl: aws.String(w.queueURL), ReceiptHandle: message.ReceiptHandle, VisibilityTimeout: w.visibilityTimeout})
				cancelExtend()
				if err != nil {
					w.health.SetReady(false)
					w.metrics.VisibilityExtensionFailed()
					w.logger.Error("failed to extend inbound message visibility", "error", err)
				} else {
					w.metrics.VisibilityExtended()
				}
			case <-stop:
				return
			case <-heartbeatCtx.Done():
				return
			}
		}
	}()
	return w.processMessage(ctx, message)
}

func (w *InboundWorker) processMessage(ctx context.Context, message types.Message) error {
	if message.Body == nil {
		return fmt.Errorf("inbound SQS message body is empty")
	}
	notification, err := inbound.DecodeAndVerifySNS(ctx, []byte(*message.Body), w.awsRegion, w.snsTopicArn)
	if err != nil {
		if errors.Is(err, sns.ErrStaleNotification) || errors.Is(err, sns.ErrInvalidNotification) {
			return fmt.Errorf("%w: verify SNS message: %v", ErrDiscardInboundMessage, err)
		}
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
	return w.inboundService.ProcessEmailWithMessageIDAndAttachments(ctx, teamID, messageID, parsed.From, routingRecipients, parsed.Subject, parsed.Text, parsed.HTML, parsed.Headers, parsed.Attachments, rawS3Key)
}

func (w *InboundWorker) deleteMessage(ctx context.Context, message types.Message) error {
	if message.ReceiptHandle == nil {
		return nil
	}
	if _, err := w.sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(w.queueURL),
		ReceiptHandle: message.ReceiptHandle,
	}); err != nil {
		return err
	}
	return nil
}

func (w *InboundWorker) readRawEmail(ctx context.Context, bucket, key string) (string, error) {
	object, err := w.s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return "", fmt.Errorf("read inbound S3 object: %w", err)
	}
	defer func() { _ = object.Body.Close() }()
	content, err := io.ReadAll(io.LimitReader(object.Body, maxInboundMessageSize+1))
	if err != nil {
		return "", fmt.Errorf("read inbound S3 content: %w", err)
	}
	if len(content) > maxInboundMessageSize {
		return "", fmt.Errorf("inbound S3 object is too large")
	}
	return string(content), nil
}
