package app

import (
	"fmt"
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
	return nil
}
