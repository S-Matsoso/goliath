// Package health reports whether the service is alive. It is the reference
// module for future feature modules.
package health

// Status is the health payload returned by the module.
type Status struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// Service produces health information. It has no HTTP knowledge.
type Service struct {
	serviceName string
}

// NewService returns a health service reporting the given service name.
func NewService(serviceName string) *Service {
	return &Service{serviceName: serviceName}
}

// Status returns the current health status. It stays cheap and dependency-free
// so it remains valid as future modules add real dependencies.
func (s *Service) Status() Status {
	return Status{Status: "ok", Service: s.serviceName}
}
