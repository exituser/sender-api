package inbound

import (
	"strings"
	"testing"
)

func TestParseRawMessagePlainText(t *testing.T) {
	message, err := ParseRawMessage("From: Sender <sender@example.com>\nTo: One <one@example.com>, two@example.com\nSubject: =?UTF-8?Q?Test_=E2=9C=93?=\nX-Trace: abc\n\nplain body")
	if err != nil {
		t.Fatal(err)
	}
	if message.From != "sender@example.com" || len(message.To) != 2 || message.Subject != "Test ✓" {
		t.Fatalf("unexpected parsed message: %#v", message)
	}
	if message.Text != "plain body" || message.HTML != "" || message.Headers["X-Trace"] != "abc" {
		t.Fatalf("unexpected MIME fields: %#v", message)
	}
}

func TestParseRawMessageHTML(t *testing.T) {
	message, err := ParseRawMessage("From: sender@example.com\nTo: recipient@example.com\nContent-Type: text/html; charset=utf-8\n\n<p>Hello</p>")
	if err != nil {
		t.Fatal(err)
	}
	if message.Text != "" || message.HTML != "<p>Hello</p>" {
		t.Fatalf("unexpected bodies: %#v", message)
	}
}

func TestParseRawMessageMultipartAlternative(t *testing.T) {
	message, err := ParseRawMessage(strings.Join([]string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"Content-Type: multipart/alternative; boundary=alt",
		"",
		"--alt",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Plain version",
		"--alt",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>HTML version</p>",
		"--alt--",
		"",
	}, "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if message.Text != "Plain version" || message.HTML != "<p>HTML version</p>" {
		t.Fatalf("unexpected multipart bodies: %#v", message)
	}
}

func TestParseRawMessageAttachment(t *testing.T) {
	message, err := ParseRawMessage(strings.Join([]string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"Content-Type: multipart/mixed; boundary=mixed",
		"",
		"--mixed",
		"Content-Type: text/plain",
		"",
		"Message body",
		"--mixed",
		"Content-Type: text/plain; name=note.txt",
		"Content-Disposition: attachment; filename=note.txt",
		"Content-ID: <attachment-1>",
		"Content-Transfer-Encoding: base64",
		"",
		"aGVsbG8=",
		"--mixed--",
		"",
	}, "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if message.Text != "Message body" || len(message.Attachments) != 1 {
		t.Fatalf("unexpected attachment result: %#v", message)
	}
	attachment := message.Attachments[0]
	if attachment.Filename != "note.txt" || attachment.ContentType != "text/plain" || attachment.ContentID != "attachment-1" || string(attachment.Content) != "hello" {
		t.Fatalf("unexpected attachment: %#v", attachment)
	}
}

func TestParseRawMessageRejectsInvalidInput(t *testing.T) {
	if _, err := ParseRawMessage("not an email message"); err == nil {
		t.Fatal("expected malformed email to be rejected")
	}
	if _, err := ParseRawMessage("From: sender@example.com\nTo: recipient@example.com\nContent-Transfer-Encoding: base64\n\nnot base64!"); err == nil {
		t.Fatal("expected invalid transfer encoding to be rejected")
	}
}

func TestParseRawMessageLimitsAttachments(t *testing.T) {
	tooLarge := "From: sender@example.com\nTo: recipient@example.com\nContent-Disposition: attachment; filename=large.bin\n\n" + strings.Repeat("a", maxInboundAttachmentSize+1)
	if _, err := ParseRawMessage(tooLarge); err == nil {
		t.Fatal("expected oversized attachment to be rejected")
	}

	parts := []string{"From: sender@example.com", "To: recipient@example.com", "Content-Type: multipart/mixed; boundary=limit", ""}
	for i := 0; i < maxInboundAttachments+1; i++ {
		parts = append(parts, "--limit", "Content-Disposition: attachment; filename=file.txt", "", "x")
	}
	parts = append(parts, "--limit--", "")
	if _, err := ParseRawMessage(strings.Join(parts, "\r\n")); err == nil {
		t.Fatal("expected excess attachment count to be rejected")
	}
}

func TestRecipientDomain(t *testing.T) {
	if got := RecipientDomain("One <ONE@Example.COM>"); got != "example.com" {
		t.Fatalf("expected normalized domain, got %q", got)
	}
}
