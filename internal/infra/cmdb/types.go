package cmdb

// CMDBServiceDTO infra 自有 YAML 数据结构，不 import ai/cmdb
// Exported so app/cmdb_adapter.go can access fields for DTO→Info conversion
type CMDBServiceDTO struct {
	Name         string   `yaml:"name"`
	DisplayName  string   `yaml:"display_name,omitempty"`
	Owner        string   `yaml:"owner"`
	Team         string   `yaml:"team"`
	Cluster      string   `yaml:"cluster"`
	Env          string   `yaml:"env"`
	Region       string   `yaml:"region,omitempty"`
	Language     string   `yaml:"language,omitempty"`
	Port         int      `yaml:"port,omitempty"`
	Dependencies []string `yaml:"dependencies,omitempty"`
	Description  string   `yaml:"description,omitempty"`
	OnCall       string   `yaml:"on_call,omitempty"`
	LastDeploy   string   `yaml:"last_deploy,omitempty"`
	Tags         []string `yaml:"tags,omitempty"`
}

type HostDTO struct {
	Name        string   `yaml:"name"`
	Service     string   `yaml:"service"`
	IP          string   `yaml:"ip"`
	Node        string   `yaml:"node,omitempty"`
	Cluster     string   `yaml:"cluster"`
	Env         string   `yaml:"env"`
	Status      string   `yaml:"status"`
	LastRestart string   `yaml:"last_restart,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
}

type cmdbFile struct {
	Services []CMDBServiceDTO `yaml:"services"`
	Hosts    []HostDTO        `yaml:"hosts,omitempty"`
}
