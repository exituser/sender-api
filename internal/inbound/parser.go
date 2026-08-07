package inbound

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
)

const (
	maxInboundMIMEMessageSize = 10 << 20
	maxInboundHeaderSize      = 64 << 10
	maxInboundMIMEParts       = 100
	maxInboundMIMEDepth       = 8
	maxInboundAttachments     = 10
	maxInboundAttachmentSize  = 5 << 20
	maxInboundTextSize        = 1 << 20
)

// Attachment is a bounded copy of an inbound MIME attachment. Content is kept
// as bytes so callers can serialize it without re-decoding transfer encodings.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ContentID   string `json:"content_id,omitempty"`
	Content     []byte `json:"content"`
}

type ParsedMessage struct {
	From        string
	To          []string
	Subject     string
	Text        string
	HTML        string
	Headers     map[string]string
	Attachments []Attachment
}

func ParseRawMessage(content string) (*ParsedMessage, error) {
	if len(content) > maxInboundMIMEMessageSize {
		return nil, fmt.Errorf("raw email exceeds %d byte limit", maxInboundMIMEMessageSize)
	}

	message, err := mail.ReadMessage(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("read raw email: %w", err)
	}
	fromAddress, err := mail.ParseAddress(message.Header.Get("From"))
	if err != nil {
		return nil, fmt.Errorf("parse sender: %w", err)
	}
	recipients, err := mail.ParseAddressList(message.Header.Get("To"))
	if err != nil || len(recipients) == 0 {
		return nil, fmt.Errorf("parse recipients")
	}

	parsed := &ParsedMessage{
		From:    fromAddress.Address,
		Subject: decodeHeader(message.Header.Get("Subject")),
		Headers: decodeHeaders(message.Header),
		To:      make([]string, 0, len(recipients)),
	}
	for _, recipient := range recipients {
		parsed.To = append(parsed.To, recipient.Address)
	}
	if parsed.Headers == nil {
		return nil, fmt.Errorf("inbound headers exceed %d byte limit", maxInboundHeaderSize)
	}

	state := mimeParseState{}
	if err := parseMIMEPart(message.Header, message.Body, parsed, &state, 0); err != nil {
		return nil, err
	}
	return parsed, nil
}

type mimeParseState struct{ parts int }

func parseMIMEPart(header mail.Header, body io.Reader, parsed *ParsedMessage, state *mimeParseState, depth int) error {
	if depth > maxInboundMIMEDepth {
		return fmt.Errorf("MIME nesting exceeds %d levels", maxInboundMIMEDepth)
	}
	state.parts++
	if state.parts > maxInboundMIMEParts {
		return fmt.Errorf("MIME part count exceeds %d", maxInboundMIMEParts)
	}

	contentType := header.Get("Content-Type")
	mediaType, params := "text/plain", map[string]string(nil)
	if contentType != "" {
		var err error
		mediaType, params, err = mime.ParseMediaType(contentType)
		if err != nil {
			return fmt.Errorf("parse MIME content type: %w", err)
		}
	}
	mediaType = strings.ToLower(mediaType)
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return fmt.Errorf("multipart message has no boundary")
		}
		reader := multipart.NewReader(body, boundary)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("read MIME part: %w", err)
			}
			err = parseMIMEPart(mail.Header(part.Header), part, parsed, state, depth+1)
			_ = part.Close()
			if err != nil {
				return err
			}
		}
	}

	filename, disposition := mimeFilename(header)
	if filename != "" || disposition == "attachment" {
		if len(parsed.Attachments) >= maxInboundAttachments {
			return fmt.Errorf("attachment count exceeds %d", maxInboundAttachments)
		}
		content, err := readMIMEBody(header, body, maxInboundAttachmentSize)
		if err != nil {
			return fmt.Errorf("read attachment: %w", err)
		}
		parsed.Attachments = append(parsed.Attachments, Attachment{
			Filename:    filename,
			ContentType: mediaType,
			ContentID:   strings.Trim(header.Get("Content-ID"), "<>"),
			Content:     content,
		})
		return nil
	}

	if mediaType != "text/plain" && mediaType != "text/html" {
		return nil
	}
	content, err := readMIMEBody(header, body, maxInboundTextSize)
	if err != nil {
		return fmt.Errorf("read %s body: %w", mediaType, err)
	}
	if mediaType == "text/plain" {
		parsed.Text = string(content)
	} else {
		parsed.HTML = string(content)
	}
	return nil
}

func mimeFilename(header mail.Header) (string, string) {
	disposition, params, err := mime.ParseMediaType(header.Get("Content-Disposition"))
	if err == nil {
		if filename := params["filename"]; filename != "" {
			return filename, strings.ToLower(disposition)
		}
	}
	_, params, err = mime.ParseMediaType(header.Get("Content-Type"))
	if err == nil {
		return params["name"], strings.ToLower(disposition)
	}
	return "", strings.ToLower(disposition)
}

func readMIMEBody(header mail.Header, body io.Reader, limit int) ([]byte, error) {
	decoded := io.Reader(body)
	switch strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding"))) {
	case "", "7bit", "8bit", "binary":
	case "base64":
		decoded = base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		decoded = quotedprintable.NewReader(body)
	default:
		return nil, fmt.Errorf("unsupported content-transfer-encoding")
	}
	content, err := io.ReadAll(io.LimitReader(decoded, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(content) > limit {
		return nil, fmt.Errorf("content exceeds %d byte limit", limit)
	}
	return content, nil
}

func decodeHeaders(headers mail.Header) map[string]string {
	decoder := new(mime.WordDecoder)
	result := make(map[string]string, len(headers))
	total := 0
	for key, values := range headers {
		decoded := make([]string, 0, len(values))
		for _, value := range values {
			decoded = append(decoded, decodeHeaderWith(decoder, value))
		}
		value := strings.Join(decoded, ", ")
		total += len(key) + len(value)
		if total > maxInboundHeaderSize {
			return nil
		}
		result[key] = value
	}
	return result
}

func decodeHeader(value string) string { return decodeHeaderWith(new(mime.WordDecoder), value) }

func decodeHeaderWith(decoder *mime.WordDecoder, value string) string {
	decoded, err := decoder.DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func RecipientDomain(address string) string {
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
