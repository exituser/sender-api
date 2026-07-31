package inbound

import (
	"fmt"
	"net/mail"
	"strings"
)

type ParsedMessage struct {
	From    string
	To      []string
	Subject string
}

func ParseRawMessage(content string) (*ParsedMessage, error) {
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
	to := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		to = append(to, recipient.Address)
	}
	return &ParsedMessage{
		From:    fromAddress.Address,
		To:      to,
		Subject: message.Header.Get("Subject"),
	}, nil
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
