package cmdb

// ServiceInfo domain model — ai/ 层使用的数据结构
type ServiceInfo struct {
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name,omitempty"`
	Owner        string   `json:"owner"`
	Team         string   `json:"team"`
	Cluster      string   `json:"cluster"`
	Env          string   `json:"env"`
	Region       string   `json:"region,omitempty"`
	Language     string   `json:"language,omitempty"`
	Port         int      `json:"port,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	Dependents   []string `json:"dependents,omitempty"`
	Description  string   `json:"description,omitempty"`
	OnCall       string   `json:"on_call,omitempty"`
	LastDeploy   string   `json:"last_deploy,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

// ServiceRepository CMDB 服务仓库接口
// ai/ 层只依赖此接口，不依赖 infra 实现
type ServiceRepository interface {
	GetService(name string) (*ServiceInfo, bool)
	SearchServices(keyword string, limit int) []ServiceInfo
	ListServicesByCluster(cluster string) []ServiceInfo
	ListServicesByTeam(team string) []ServiceInfo
	GetDependents(name string) []string
	ListAll() []ServiceInfo
	CreateService(svc ServiceInfo) error
	UpdateService(name string, svc ServiceInfo) error
	DeleteService(name string) error
}
