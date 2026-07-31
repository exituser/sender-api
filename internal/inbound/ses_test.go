package inbound

import "testing"

func TestDecodeNotificationReadsSESReceiptEnvelope(t *testing.T) {
	notification, err := DecodeNotification([]byte(`{
        "type":"Received",
        "messageId":"ses-message-id",
        "receipt":{"recipients":["inbox@example.com"],"action":{"type":"S3","bucketName":"sender-api-inbound","objectKey":"raw/message"}},
        "content":"From: sender@example.net\nTo: forged@example.org\n\nbody"
    }`))
	if err != nil {
		t.Fatal(err)
	}
	if notification.Type != "Received" || notification.MessageID != "ses-message-id" {
		t.Fatalf("unexpected notification: %#v", notification)
	}
	if len(notification.Receipt.Recipients) != 1 || notification.Receipt.Recipients[0] != "inbox@example.com" {
		t.Fatalf("expected envelope recipient, got %#v", notification.Receipt.Recipients)
	}
	if notification.Receipt.Action.ObjectKey != "raw/message" {
		t.Fatalf("expected S3 object key, got %q", notification.Receipt.Action.ObjectKey)
	}
}

func TestDecodeNotificationNormalizesCanonicalSESFields(t *testing.T) {
	notification, err := DecodeNotification([]byte(`{
        "notificationType":"Received",
        "mail":{"messageId":"ses-mail-id"},
        "receipt":{"recipients":["inbox@example.com"]},
        "headers":[{"name":"X-Test","value":"value"}],
        "content":"From: sender@example.net\nTo: inbox@example.com\n\nbody"
    }`))
	if err != nil {
		t.Fatal(err)
	}
	if notification.Type != "Received" || notification.MessageID != "ses-mail-id" {
		t.Fatalf("expected normalized SES fields, got type=%q message_id=%q", notification.Type, notification.MessageID)
	}
}
