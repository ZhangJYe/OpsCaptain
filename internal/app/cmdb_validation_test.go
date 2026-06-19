package app

import (
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/cmdb"
)

func validServiceInfo() cmdb.ServiceInfo {
	return cmdb.ServiceInfo{
		Name:    "testservice",
		Owner:   "test-owner",
		Team:    "test-team",
		Cluster: "prod-01",
		Env:     "production",
	}
}

func TestValidateService_Success(t *testing.T) {
	if err := validateService(validServiceInfo()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateService_NameRequired(t *testing.T) {
	svc := validServiceInfo()
	svc.Name = ""
	if err := validateService(svc); err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required', got %v", err)
	}
}

func TestValidateService_NameMaxLength(t *testing.T) {
	svc := validServiceInfo()
	svc.Name = strings.Repeat("a", 129)
	if err := validateService(svc); err == nil || !strings.Contains(err.Error(), "at most 128") {
		t.Errorf("expected max length error, got %v", err)
	}
}

func TestValidateService_NameInvalidChars(t *testing.T) {
	svc := validServiceInfo()
	svc.Name = "Test_Service"
	if err := validateService(svc); err == nil || !strings.Contains(err.Error(), "lowercase alphanumeric") {
		t.Errorf("expected name format error, got %v", err)
	}
}

func TestValidateService_OwnerRequired(t *testing.T) {
	svc := validServiceInfo()
	svc.Owner = ""
	if err := validateService(svc); err == nil || !strings.Contains(err.Error(), "owner is required") {
		t.Errorf("expected 'owner is required', got %v", err)
	}
}

func TestValidateService_TeamRequired(t *testing.T) {
	svc := validServiceInfo()
	svc.Team = ""
	if err := validateService(svc); err == nil || !strings.Contains(err.Error(), "team is required") {
		t.Errorf("expected 'team is required', got %v", err)
	}
}

func TestValidateService_ClusterRequired(t *testing.T) {
	svc := validServiceInfo()
	svc.Cluster = ""
	if err := validateService(svc); err == nil || !strings.Contains(err.Error(), "cluster is required") {
		t.Errorf("expected 'cluster is required', got %v", err)
	}
}

func TestValidateService_EnvRequired(t *testing.T) {
	svc := validServiceInfo()
	svc.Env = ""
	if err := validateService(svc); err == nil || !strings.Contains(err.Error(), "env is required") {
		t.Errorf("expected 'env is required', got %v", err)
	}
}

func TestValidateService_HyphensAllowed(t *testing.T) {
	svc := validServiceInfo()
	svc.Name = "my-service-v2"
	if err := validateService(svc); err != nil {
		t.Errorf("expected no error for hyphenated name, got %v", err)
	}
}
