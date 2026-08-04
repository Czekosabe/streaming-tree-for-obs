package output

import (
	"context"
	"time"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// Service holds the output-configuration use cases.
type Service struct {
	repo Repository
	now  Clock
}

// ServiceOption customises a Service, mainly for tests.
type ServiceOption func(*Service)

// WithClock overrides the time source.
func WithClock(clock Clock) ServiceOption {
	return func(s *Service) { s.now = clock }
}

// NewService builds a Service around a repository.
func NewService(repo Repository, opts ...ServiceOption) *Service {
	s := &Service{repo: repo, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Get returns one platform's output settings.
func (s *Service) Get(ctx context.Context, platformID string) (Settings, error) {
	return s.repo.Get(ctx, platformID)
}

// Update validates and replaces a platform's output settings.
func (s *Service) Update(ctx context.Context, platformID string, input UpdateInput) (Settings, error) {
	serverURL, err := ValidateServerURL(input.ServerURL)
	if err != nil {
		return Settings{}, err
	}

	updatedAt := s.now().UTC().Format(time.RFC3339Nano)
	return s.repo.Update(ctx, platformID, UpdateInput{
		ServerURL:   serverURL,
		AutoRestart: input.AutoRestart,
	}, updatedAt)
}
