// Package health reports whether the service is alive. It is the reference
// module for future feature modules.
package health

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Status is the health payload returned by the module.
type Status struct {
	Status   string `json:"status"`
	Service  string `json:"service"`
	Database string `json:"database,omitempty"`
}

// Service produces health information. It has no HTTP knowledge.
type Service struct {
	serviceName string
	database    *gorm.DB
}

// NewService returns a health service reporting the given service name.
func NewService(serviceName string, database ...*gorm.DB) *Service {
	service := &Service{serviceName: serviceName}
	if len(database) > 0 {
		service.database = database[0]
	}
	return service
}

// Status checks the process and its database dependency without exposing
// dependency error details.
func (s *Service) Status(ctx context.Context) Status {
	status := Status{Status: "ok", Service: s.serviceName}
	if s.database == nil {
		return status
	}
	sqlDB, err := s.database.DB()
	if err != nil {
		status.Status = "degraded"
		status.Database = "unavailable"
		return status
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(checkCtx); err != nil {
		status.Status = "degraded"
		status.Database = "unavailable"
		return status
	}
	status.Database = "healthy"
	return status
}
