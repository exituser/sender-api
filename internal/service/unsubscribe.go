package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/validator"
)

var ErrInvalidUnsubscribeToken = errors.New("invalid unsubscribe token")

type unsubscribeClaims struct {
	TeamID string `json:"team_id"`
	Email  string `json:"email"`
}

// UnsubscribeSigner creates opaque, authenticated tokens. The token contains
// no database identifier and can be rotated by changing the configured secret.
// It is intentionally separate from the Supabase service key.
type UnsubscribeSigner struct {
	block cipher.AEAD
	base  string
}

func NewUnsubscribeSigner(secret, baseURL string) (*UnsubscribeSigner, error) {
	secret = strings.TrimSpace(secret)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if len(secret) < 32 {
		return nil, fmt.Errorf("unsubscribe signing secret must be at least 32 characters")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" {
		return nil, fmt.Errorf("unsubscribe base URL must be an absolute http(s) URL")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create unsubscribe cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create unsubscribe AEAD: %w", err)
	}
	return &UnsubscribeSigner{block: aead, base: baseURL}, nil
}

func (s *UnsubscribeSigner) Token(teamID uuid.UUID, email string) (string, error) {
	if s == nil || s.block == nil {
		return "", ErrInvalidUnsubscribeToken
	}
	email = domain.NormalizeEmail(email)
	if teamID == uuid.Nil || !validator.IsValidEmail(email) {
		return "", ErrInvalidUnsubscribeToken
	}
	payload, err := json.Marshal(unsubscribeClaims{TeamID: teamID.String(), Email: email})
	if err != nil {
		return "", fmt.Errorf("encode unsubscribe claims: %w", err)
	}
	nonce := make([]byte, s.block.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate unsubscribe nonce: %w", err)
	}
	sealed := s.block.Seal(nil, nonce, payload, nil)
	token := append(nonce, sealed...)
	return base64.RawURLEncoding.EncodeToString(token), nil
}

func (s *UnsubscribeSigner) parse(token string) (*unsubscribeClaims, error) {
	if s == nil || s.block == nil || token == "" || len(token) > 4096 {
		return nil, ErrInvalidUnsubscribeToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) <= s.block.NonceSize() {
		return nil, ErrInvalidUnsubscribeToken
	}
	nonce, sealed := raw[:s.block.NonceSize()], raw[s.block.NonceSize():]
	payload, err := s.block.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, ErrInvalidUnsubscribeToken
	}
	var claims unsubscribeClaims
	if json.Unmarshal(payload, &claims) != nil {
		return nil, ErrInvalidUnsubscribeToken
	}
	teamID, err := uuid.Parse(claims.TeamID)
	if err != nil || teamID == uuid.Nil || !validator.IsValidEmail(claims.Email) {
		return nil, ErrInvalidUnsubscribeToken
	}
	claims.Email = domain.NormalizeEmail(claims.Email)
	return &claims, nil
}

func (s *UnsubscribeSigner) URL(teamID uuid.UUID, email string) (string, error) {
	token, err := s.Token(teamID, email)
	if err != nil {
		return "", err
	}
	return s.base + "/api/v1/unsubscribe/" + url.PathEscape(token), nil
}

type contactSubscriptionWriter interface {
	SetSubscribedByEmail(ctx context.Context, teamID uuid.UUID, email string, subscribed bool) (bool, error)
}

type UnsubscribeService struct {
	signer          *UnsubscribeSigner
	contactRepo     domain.ContactRepository
	suppressionRepo domain.SuppressionRepository
}

func NewUnsubscribeService(signer *UnsubscribeSigner, contactRepo domain.ContactRepository, suppressionRepo domain.SuppressionRepository) *UnsubscribeService {
	return &UnsubscribeService{signer: signer, contactRepo: contactRepo, suppressionRepo: suppressionRepo}
}

func (s *UnsubscribeService) LandingURL(teamID uuid.UUID, email string) (string, error) {
	if s == nil || s.signer == nil {
		return "", ErrInvalidUnsubscribeToken
	}
	return s.signer.URL(teamID, email)
}

func (s *UnsubscribeService) Unsubscribe(ctx context.Context, token string) error {
	if s == nil || s.signer == nil {
		return ErrInvalidUnsubscribeToken
	}
	claims, err := s.signer.parse(token)
	if err != nil {
		return err
	}
	teamID, _ := uuid.Parse(claims.TeamID)

	if writer, ok := s.contactRepo.(contactSubscriptionWriter); ok {
		if _, err := writer.SetSubscribedByEmail(ctx, teamID, claims.Email, false); err != nil {
			return fmt.Errorf("unsubscribe contact: %w", err)
		}
	} else if s.contactRepo != nil {
		return fmt.Errorf("contact subscription updates are not configured")
	}
	if s.suppressionRepo != nil {
		if err := s.suppressionRepo.Upsert(ctx, &domain.Suppression{
			ID:     uuid.New(),
			TeamID: teamID,
			Email:  claims.Email,
			Reason: domain.SuppressionReasonUnsubscribe,
		}); err != nil {
			return fmt.Errorf("record unsubscribe suppression: %w", err)
		}
	}
	return nil
}
