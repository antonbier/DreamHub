package peer

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/apitoken"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

const (
	// CodeLife is how long a pairing code can be used.
	CodeLife = 10 * time.Minute
	// MaxAttempts wrong codes end the code: eight digits are safe against a
	// few guesses, not against someone who may try forever.
	MaxAttempts = 5
)

// Service is the answering end: the DreamHub on the box that is being
// paired. It shows a code on request and turns the right code into a token.
type Service struct {
	Tokens *apitoken.Service
	// Announce shows the code on the box's TV; may be nil. Its error is not
	// fatal — the code is on the status page anyway.
	Announce func(ctx context.Context, code string) error
	Now      func() time.Time

	mu       sync.Mutex
	code     string
	expires  time.Time
	attempts int
	owner    store.User
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Code is a pairing code as shown to the administrator.
type Code struct {
	// Code is eight digits, grouped for reading ("1234 5678").
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
	// OnTV: the code was also put on the box's TV.
	OnTV bool `json:"onTv"`
}

// Start shows a new code (replacing an older one). The token a successful
// pairing creates belongs to owner, the administrator who asked.
func (s *Service) Start(ctx context.Context, owner store.User) (Code, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(100_000_000))
	if err != nil {
		return Code{}, fmt.Errorf("peer: generating a code: %w", err)
	}
	digits := fmt.Sprintf("%08d", n.Int64())
	s.mu.Lock()
	s.code, s.expires, s.attempts, s.owner = digits, s.now().Add(CodeLife), 0, owner
	exp := s.expires
	s.mu.Unlock()

	out := Code{Code: digits[:4] + " " + digits[4:], ExpiresAt: exp}
	if s.Announce != nil && s.Announce(ctx, out.Code) == nil {
		out.OnTV = true
	}
	return out, nil
}

// Active reports whether a code can be used right now.
func (s *Service) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeLocked()
}

func (s *Service) activeLocked() bool {
	return s.code != "" && s.now().Before(s.expires)
}

// Cancel ends the current code.
func (s *Service) Cancel() {
	s.mu.Lock()
	s.code = ""
	s.mu.Unlock()
}

func normalize(code string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, code)
}

// Redeem turns the right code into a token named after the hub. A code is
// good once: it ends with its use, and with the MaxAttempts-th wrong try.
func (s *Service) Redeem(ctx context.Context, code, hubName string) (string, error) {
	s.mu.Lock()
	if !s.activeLocked() {
		s.code = ""
		s.mu.Unlock()
		return "", ErrNotPairing
	}
	if subtle.ConstantTimeCompare([]byte(normalize(code)), []byte(s.code)) != 1 {
		s.attempts++
		if s.attempts >= MaxAttempts {
			s.code = ""
		}
		s.mu.Unlock()
		return "", ErrBadCode
	}
	owner := s.owner
	s.code = ""
	s.mu.Unlock()

	hubName = strings.TrimSpace(hubName)
	if hubName == "" {
		hubName = "hub"
	}
	created, err := s.Tokens.Create(ctx, owner, "Hub: "+hubName)
	if err != nil {
		return "", err
	}
	return created.Secret, nil
}
