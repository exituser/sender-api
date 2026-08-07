package sns

import (
	"errors"
	"testing"
	"time"
)

func TestStringToSignUsesSNSCanonicalOrder(t *testing.T) {
	got := StringToSign(Notification{
		Type:      "Notification",
		Message:   "payload",
		MessageID: "message-id",
		Subject:   "subject",
		Timestamp: "2026-07-31T12:00:00Z",
		TopicArn:  "arn:aws:sns:eu-west-1:123:topic",
	})
	want := "Message\npayload\nMessageId\nmessage-id\nSubject\nsubject\nTimestamp\n2026-07-31T12:00:00Z\nTopicArn\narn:aws:sns:eu-west-1:123:topic\nType\nNotification\n"
	if got != want {
		t.Fatalf("unexpected canonical string:\n%q", got)
	}
}

func TestStringToSignUsesConfirmationFields(t *testing.T) {
	got := StringToSign(Notification{
		Type:         "SubscriptionConfirmation",
		Message:      "confirm",
		MessageID:    "message-id",
		SubscribeURL: "https://sns.eu-west-1.amazonaws.com/?Action=ConfirmSubscription",
		Timestamp:    "2026-07-31T12:00:00Z",
		Token:        "token",
		TopicArn:     "arn:aws:sns:eu-west-1:123:topic",
	})
	want := "Message\nconfirm\nMessageId\nmessage-id\nSubscribeURL\nhttps://sns.eu-west-1.amazonaws.com/?Action=ConfirmSubscription\nTimestamp\n2026-07-31T12:00:00Z\nToken\ntoken\nTopicArn\narn:aws:sns:eu-west-1:123:topic\nType\nSubscriptionConfirmation\n"
	if got != want {
		t.Fatalf("unexpected confirmation canonical string:\n%q", got)
	}
}

func TestConfirmSubscriptionRejectsNonAmazonURLBeforeNetwork(t *testing.T) {
	err := ConfirmSubscription(t.Context(), Notification{
		Type:         "SubscriptionConfirmation",
		SubscribeURL: "https://example.com/confirm?Action=ConfirmSubscription&Token=x&TopicArn=arn",
	}, "eu-west-1")
	if err == nil {
		t.Fatal("expected unsafe SNS confirmation URL to be rejected")
	}
}

func TestVerifyNotificationRejectsStaleAndUnsafeMessagesBeforeNetwork(t *testing.T) {
	for _, test := range []struct {
		name string
		item Notification
	}{
		{name: "stale", item: Notification{Type: "Notification", Message: "x", MessageID: "id", Timestamp: time.Now().Add(-time.Hour).Format(time.RFC3339), TopicArn: "arn", Signature: "x", SignatureVersion: "2", SigningCertURL: "https://sns.eu-west-1.amazonaws.com/cert.pem"}},
		{name: "unsafe certificate host", item: Notification{Type: "Notification", Message: "x", MessageID: "id", Timestamp: time.Now().Format(time.RFC3339), TopicArn: "arn", Signature: "x", SignatureVersion: "2", SigningCertURL: "https://example.com/cert.pem"}},
		{name: "missing signature version", item: Notification{Type: "Notification", Message: "x", MessageID: "id", Timestamp: time.Now().Format(time.RFC3339), TopicArn: "arn", Signature: "x", SigningCertURL: "https://sns.eu-west-1.amazonaws.com/cert.pem"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := VerifyNotification(t.Context(), test.item, "eu-west-1", time.Now())
			if err == nil {
				t.Fatal("expected SNS notification to be rejected")
			}
			if test.name == "stale" && !errors.Is(err, ErrStaleNotification) {
				t.Fatalf("expected stale notification error, got %v", err)
			}
		})
	}
}

func TestVerifyNotificationForQueueDoesNotDropDelayedMessageAsStale(t *testing.T) {
	notification := Notification{
		Type:             "Notification",
		Message:          "x",
		MessageID:        "id",
		Timestamp:        time.Now().Add(-time.Hour).Format(time.RFC3339),
		TopicArn:         "arn",
		Signature:        "x",
		SignatureVersion: "2",
		SigningCertURL:   "https://example.com/cert.pem",
	}
	err := VerifyNotificationForQueue(t.Context(), notification, "eu-west-1", time.Now())
	if errors.Is(err, ErrStaleNotification) {
		t.Fatalf("queue verification must not reject delayed notification as stale: %v", err)
	}
}
