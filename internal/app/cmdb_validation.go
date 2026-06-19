package app

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	"SuperBizAgent/internal/ai/cmdb"
)

var validNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

func validateService(svc cmdb.ServiceInfo) error {
	if strings.TrimSpace(svc.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(svc.Name) > 128 {
		return fmt.Errorf("name must be at most 128 characters")
	}
	if !validNameRe.MatchString(svc.Name) {
		return fmt.Errorf("name must be lowercase alphanumeric with hyphens, got %q", svc.Name)
	}
	if strings.TrimSpace(svc.Owner) == "" {
		return fmt.Errorf("owner is required")
	}
	if strings.TrimSpace(svc.Team) == "" {
		return fmt.Errorf("team is required")
	}
	if strings.TrimSpace(svc.Cluster) == "" {
		return fmt.Errorf("cluster is required")
	}
	if strings.TrimSpace(svc.Env) == "" {
		return fmt.Errorf("env is required")
	}
	// Prevent self-dependency
	for _, dep := range svc.Dependencies {
		if dep == svc.Name {
			return fmt.Errorf("service cannot depend on itself")
		}
	}
	return nil
}

// validateServiceDeps checks that all dependencies exist in the service list.
// Called after basic validation, requires access to all existing services.
func validateServiceDeps(svc cmdb.ServiceInfo, existingServices []cmdb.ServiceInfo) error {
	existing := make(map[string]bool, len(existingServices))
	for _, s := range existingServices {
		existing[s.Name] = true
	}
	for _, dep := range svc.Dependencies {
		if !existing[dep] {
			return fmt.Errorf("dependency %q does not exist", dep)
		}
	}
	return nil
}

var validHostStatuses = map[string]bool{
	"running":   true,
	"stopped":   true,
	"unhealthy": true,
}

func validateHost(host cmdb.HostInfo) error {
	if strings.TrimSpace(host.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(host.Service) == "" {
		return fmt.Errorf("service is required")
	}
	if strings.TrimSpace(host.IP) == "" {
		return fmt.Errorf("ip is required")
	}
	if net.ParseIP(host.IP) == nil {
		return fmt.Errorf("ip %q is not a valid IP address", host.IP)
	}
	if strings.TrimSpace(host.Cluster) == "" {
		return fmt.Errorf("cluster is required")
	}
	if strings.TrimSpace(host.Env) == "" {
		return fmt.Errorf("env is required")
	}
	if host.Status != "" && !validHostStatuses[host.Status] {
		return fmt.Errorf("status must be one of: running, stopped, unhealthy")
	}
	return nil
}

// validateHostService checks that the host's service exists.
func validateHostService(host cmdb.HostInfo, existingServices []cmdb.ServiceInfo) error {
	for _, s := range existingServices {
		if s.Name == host.Service {
			return nil
		}
	}
	return fmt.Errorf("service %q does not exist", host.Service)
}
