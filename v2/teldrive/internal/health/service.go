package health

import (
	"context"
	"fmt"
)

type Pinger interface {
	Ping(context.Context) error
}

type Status struct {
	State   string
	Version string
}

type Service struct {
	version string
	db      Pinger
}

func NewService(version string, db Pinger) *Service {
	return &Service{version: version, db: db}
}

func (s *Service) Live() Status {
	return Status{State: "ok", Version: s.version}
}

func (s *Service) Ready(ctx context.Context) (Status, error) {
	if s.db == nil {
		return Status{State: "degraded", Version: s.version}, fmt.Errorf("database is not configured")
	}
	if err := s.db.Ping(ctx); err != nil {
		return Status{State: "degraded", Version: s.version}, fmt.Errorf("ping database: %w", err)
	}
	return Status{State: "ok", Version: s.version}, nil
}
