package mailer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/smithy-go"
	"github.com/sender-api/sender-api/internal/domain"
)

// SESIdentityProvider adapts the AWS SES v2 identity API to the small port
// consumed by DomainService. Keeping the SDK behind this adapter makes the
// verification flow deterministic in unit tests and avoids leaking AWS types
// into business logic.
type SESIdentityProvider struct {
	client    *sesv2.Client
	configSet string
}

func NewSESIdentityProvider(client *sesv2.Client, configSet string) *SESIdentityProvider {
	return &SESIdentityProvider{client: client, configSet: strings.TrimSpace(configSet)}
}

func (p *SESIdentityProvider) Create(ctx context.Context, identity string) (*domain.SESIdentity, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("SES identity client is not configured")
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return nil, fmt.Errorf("SES identity is required")
	}

	input := &sesv2.CreateEmailIdentityInput{EmailIdentity: aws.String(identity)}
	if p.configSet != "" {
		input.ConfigurationSetName = aws.String(p.configSet)
	}
	output, err := p.client.CreateEmailIdentity(ctx, input)
	if err != nil {
		var apiErr smithy.APIError
		if !errors.As(err, &apiErr) || !strings.EqualFold(apiErr.ErrorCode(), "AlreadyExistsException") {
			return nil, fmt.Errorf("create SES identity: %w", err)
		}
		return p.Get(ctx, identity)
	}
	status := "pending"
	if output.VerifiedForSendingStatus {
		status = "success"
	}
	return mapSESIdentity(output.VerifiedForSendingStatus, status, output.DkimAttributes), nil
}

func (p *SESIdentityProvider) Get(ctx context.Context, identity string) (*domain.SESIdentity, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("SES identity client is not configured")
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return nil, fmt.Errorf("SES identity is required")
	}
	output, err := p.client.GetEmailIdentity(ctx, &sesv2.GetEmailIdentityInput{
		EmailIdentity: aws.String(identity),
	})
	if err != nil {
		return nil, fmt.Errorf("get SES identity: %w", err)
	}
	status := ""
	if output.VerificationStatus != "" {
		status = strings.ToLower(string(output.VerificationStatus))
	}
	return &domain.SESIdentity{
		VerifiedForSending: output.VerifiedForSendingStatus,
		VerificationStatus: status,
		DKIMStatus:         dkimStatus(output.DkimAttributes),
		DKIMTokens:         dkimTokens(output.DkimAttributes),
		SigningHostedZone:  signingHostedZone(output.DkimAttributes),
	}, nil
}

func mapSESIdentity(verified bool, verificationStatus string, attributes *types.DkimAttributes) *domain.SESIdentity {
	return &domain.SESIdentity{
		VerifiedForSending: verified,
		VerificationStatus: verificationStatus,
		DKIMStatus:         dkimStatus(attributes),
		DKIMTokens:         dkimTokens(attributes),
		SigningHostedZone:  signingHostedZone(attributes),
	}
}

func dkimStatus(attributes *types.DkimAttributes) string {
	if attributes == nil || attributes.Status == "" {
		return ""
	}
	return strings.ToLower(string(attributes.Status))
}

func dkimTokens(attributes *types.DkimAttributes) []string {
	if attributes == nil {
		return nil
	}
	return append([]string(nil), attributes.Tokens...)
}

func signingHostedZone(attributes *types.DkimAttributes) string {
	if attributes == nil || attributes.SigningHostedZone == nil {
		return ""
	}
	return strings.TrimSpace(*attributes.SigningHostedZone)
}
