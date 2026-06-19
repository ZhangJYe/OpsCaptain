package cmdb

import "testing"

type mockRepository struct {
	services map[string]*ServiceInfo
}

func (m *mockRepository) GetService(name string) (*ServiceInfo, bool) {
	svc, ok := m.services[name]
	return svc, ok
}

func (m *mockRepository) SearchServices(keyword string, limit int) []ServiceInfo {
	var results []ServiceInfo
	for _, svc := range m.services {
		if len(results) >= limit && limit > 0 {
			break
		}
		results = append(results, *svc)
	}
	return results
}

func (m *mockRepository) ListServicesByCluster(cluster string) []ServiceInfo {
	var results []ServiceInfo
	for _, svc := range m.services {
		if svc.Cluster == cluster {
			results = append(results, *svc)
		}
	}
	return results
}

func (m *mockRepository) ListServicesByTeam(team string) []ServiceInfo {
	var results []ServiceInfo
	for _, svc := range m.services {
		if svc.Team == team {
			results = append(results, *svc)
		}
	}
	return results
}

func (m *mockRepository) GetDependents(name string) []string { return nil }

func (m *mockRepository) ListAll() []ServiceInfo {
	var results []ServiceInfo
	for _, svc := range m.services {
		results = append(results, *svc)
	}
	return results
}

func TestFormatServiceInfo(t *testing.T) {
	svc := &ServiceInfo{
		Name:         "paymentservice",
		DisplayName:  "支付服务",
		Owner:        "张三",
		Team:         "支付团队",
		Cluster:      "prod-cluster-01",
		Env:          "production",
		Dependencies: []string{"userservice"},
		Dependents:   []string{"gateway"},
		Tags:         []string{"critical"},
	}
	formatted := FormatServiceInfo(svc)
	if formatted == "" {
		t.Error("expected non-empty formatted string")
	}
}

func TestFormatServiceList(t *testing.T) {
	services := []ServiceInfo{
		{Name: "a", Owner: "x", Team: "t1", Cluster: "c1", Env: "prod"},
		{Name: "b", Owner: "y", Team: "t2", Cluster: "c2", Env: "prod"},
	}
	formatted := FormatServiceList(services)
	if formatted == "" {
		t.Error("expected non-empty formatted string")
	}
}
