package mailer

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

type sesClientStub struct {
	input  *sesv2.SendEmailInput
	output *sesv2.SendEmailOutput
	err    error
}

func (s *sesClientStub) SendEmail(_ context.Context, input *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
	s.input = input
	return s.output, s.err
}

type sesAPIError struct {
	code  string
	fault smithy.ErrorFault
}

func (e sesAPIError) Error() string        { return e.ErrorMessage() }
func (e sesAPIError) ErrorCode() string    { return e.code }
func (e sesAPIError) ErrorMessage() string { return e.code }
func (e sesAPIError) ErrorFault() smithy.ErrorFault {
	return e.fault
}

func TestSESMailerAddsStableCorrelationTags(t *testing.T) {
	client := &sesClientStub{output: &sesv2.SendEmailOutput{MessageId: aws.String("provider-id")}}
	mailer := NewSESMailer(client, "events", slog.Default())
	attemptID := uuid.New()
	emailID := uuid.New()
	_, err := mailer.Send(context.Background(), &domain.Email{
		ID: emailID, SendAttemptID: &attemptID, From: "sender@example.com",
		To: []string{"person@example.net"}, Subject: "Hello", Text: "Body",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	tags := make(map[string]string)
	for _, tag := range client.input.EmailTags {
		tags[aws.ToString(tag.Name)] = aws.ToString(tag.Value)
	}
	if tags["sender_email_id"] != emailID.String() || tags["sender_attempt_id"] != attemptID.String() {
		t.Fatalf("unexpected correlation tags: %#v", tags)
	}
}

func TestSESMailerTreatsMissingReceiptAsUnknownOutcome(t *testing.T) {
	mailer := NewSESMailer(&sesClientStub{output: &sesv2.SendEmailOutput{}}, "", slog.Default())
	_, err := mailer.Send(context.Background(), &domain.Email{
		ID: uuid.New(), From: "sender@example.com", To: []string{"person@example.net"}, Subject: "Hello", Text: "Body",
	})
	if !domain.DeliveryOutcomeUnknown(err) {
		t.Fatalf("expected unknown outcome, got %v", err)
	}
}

func TestSESOutcomeClassification(t *testing.T) {
	if !sesOutcomeUnknown(context.DeadlineExceeded) {
		t.Fatal("deadline exceeded must be treated as unknown")
	}
	if sesOutcomeUnknown(sesAPIError{code: "MessageRejected", fault: smithy.FaultClient}) {
		t.Fatal("explicit provider rejection must be treated as definitive")
	}
	if !sesOutcomeUnknown(sesAPIError{code: "InternalServerError", fault: smithy.FaultServer}) {
		t.Fatal("provider server failure must be treated as an unknown outcome")
	}
	if !sesOutcomeUnknown(sesAPIError{code: "ThrottlingException", fault: smithy.FaultClient}) {
		t.Fatal("transient provider API failures must not trigger a duplicate send")
	}
	if !sesOutcomeUnknown(errors.New("transport closed")) {
		t.Fatal("transport error must be treated as unknown")
	}
}
