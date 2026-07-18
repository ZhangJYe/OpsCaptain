package milvus

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/frame/g"
	cli "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

var (
	clientsMu sync.Mutex
	clients   []cli.Client
)

func NewMilvusClient(ctx context.Context) (cli.Client, error) {
	cfg := MilvusConfigFromContext(ctx)
	return newMilvusClientWithConfig(ctx, cfg)
}

// OpenExistingMilvusClient opens an existing database and collection without
// creating or loading resources. Evaluation uses this path to keep production
// Milvus access read-only.
func OpenExistingMilvusClient(ctx context.Context) (cli.Client, error) {
	cfg := MilvusConfigFromContext(ctx)
	if _, err := InspectCollection(ctx); err != nil {
		return nil, fmt.Errorf("inspect existing Milvus collection: %w", err)
	}
	agentClient, err := cli.NewClient(ctx, cli.Config{
		Address: cfg.Addr,
		DBName:  cfg.DBName,
	})
	if err != nil {
		return nil, fmt.Errorf("connect to existing Milvus database: %w", err)
	}
	registerClient(agentClient)
	return agentClient, nil
}

func newMilvusClientWithConfig(ctx context.Context, cfg MilvusConfig) (cli.Client, error) {
	defaultClient, err := cli.NewClient(ctx, cli.Config{
		Address: cfg.Addr,
		DBName:  "default",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to default database: %w", err)
	}
	defer defaultClient.Close()

	databases, err := defaultClient.ListDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}
	agentDBExists := false
	for _, db := range databases {
		if db.Name == cfg.DBName {
			agentDBExists = true
			break
		}
	}
	if !agentDBExists {
		err = defaultClient.CreateDatabase(ctx, cfg.DBName)
		if err != nil {
			return nil, fmt.Errorf("failed to create agent database: %w", err)
		}
	}

	agentClient, err := cli.NewClient(ctx, cli.Config{
		Address: cfg.Addr,
		DBName:  cfg.DBName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to agent database: %w", err)
	}
	registerClient(agentClient)

	collections, err := agentClient.ListCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}

	bizCollectionExists := false
	for _, collection := range collections {
		if collection.Name == cfg.CollectionName {
			bizCollectionExists = true
			break
		}
	}

	if !bizCollectionExists {
		collSchema := &entity.Schema{
			CollectionName: cfg.CollectionName,
			Description:    "Business knowledge collection",
			Fields:         BuildMilvusFields(cfg),
		}

		err = agentClient.CreateCollection(ctx, collSchema, entity.DefaultShardNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to create biz collection: %w", err)
		}

		vectorIndex, err := buildVectorIndex(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create vector index: %w", err)
		}
		err = agentClient.CreateIndex(ctx, cfg.CollectionName, "vector", vectorIndex, false)
		if err != nil {
			return nil, fmt.Errorf("failed to create vector index: %w", err)
		}
	}

	err = agentClient.LoadCollection(ctx, cfg.CollectionName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to load biz collection: %w", err)
	}

	return agentClient, nil
}

func CloseAllClients() error {
	clientsMu.Lock()
	c := clients
	clients = nil
	clientsMu.Unlock()

	var errs []string
	for _, cl := range c {
		if cl == nil {
			continue
		}
		if err := cl.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("milvus close failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

func PingMilvus(ctx context.Context) error {
	cfg := MilvusConfigFromContext(ctx)
	c, err := cli.NewClient(ctx, cli.Config{
		Address: cfg.Addr,
		DBName:  "default",
	})
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer c.Close()

	if _, err := c.ListDatabases(ctx); err != nil {
		return fmt.Errorf("list databases failed: %w", err)
	}
	return nil
}

func registerClient(c cli.Client) {
	if c == nil {
		return
	}
	clientsMu.Lock()
	clients = append(clients, c)
	clientsMu.Unlock()
}

func buildVectorIndex(cfg MilvusConfig) (entity.Index, error) {
	metricType, err := resolveMetricType(cfg.MetricType)
	if err != nil {
		return nil, err
	}

	switch strings.ToUpper(strings.TrimSpace(cfg.IndexType)) {
	case "HNSW":
		g.Log().Infof(context.Background(), "creating Milvus HNSW index, metric=%s, m=%d, efConstruction=%d", string(metricType), cfg.HNSWM, cfg.HNSWEfConstruction)
		return entity.NewIndexHNSW(metricType, cfg.HNSWM, cfg.HNSWEfConstruction)
	case "AUTOINDEX", "AUTO":
		g.Log().Infof(context.Background(), "creating Milvus AUTOINDEX, metric=%s", string(metricType))
		return entity.NewIndexAUTOINDEX(metricType)
	default:
		return nil, fmt.Errorf("unsupported milvus.index_type: %s", cfg.IndexType)
	}
}

func resolveMetricType(raw string) (entity.MetricType, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "IP":
		return entity.IP, nil
	case "L2":
		return entity.L2, nil
	case "COSINE":
		return entity.COSINE, nil
	default:
		return "", fmt.Errorf("unsupported milvus.metric_type: %s", raw)
	}
}

func BuildMilvusFields(cfg MilvusConfig) []*entity.Field {
	return []*entity.Field{
		{
			Name:     "id",
			DataType: entity.FieldTypeVarChar,
			TypeParams: map[string]string{
				"max_length": "256",
			},
			PrimaryKey: true,
		},
		{
			Name:     "vector",
			DataType: entity.FieldTypeFloatVector,
			TypeParams: map[string]string{
				"dim": strconv.Itoa(cfg.VectorDimension),
			},
		},
		{
			Name:     "content",
			DataType: entity.FieldTypeVarChar,
			TypeParams: map[string]string{
				"max_length": "8192",
			},
		},
		{
			Name:     "metadata",
			DataType: entity.FieldTypeJSON,
		},
	}
}

type CollectionReport struct {
	Collection string
	SchemaOK   bool
	DocCount   int64
}

func InspectCollection(ctx context.Context) (CollectionReport, error) {
	cfg := MilvusConfigFromContext(ctx)
	report := CollectionReport{Collection: cfg.CollectionName}

	defaultClient, err := cli.NewClient(ctx, cli.Config{
		Address: cfg.Addr,
		DBName:  "default",
	})
	if err != nil {
		return report, err
	}
	defer defaultClient.Close()

	databases, err := defaultClient.ListDatabases(ctx)
	if err != nil {
		return report, fmt.Errorf("failed to list databases: %w", err)
	}
	agentDBExists := false
	for _, db := range databases {
		if db.Name == cfg.DBName {
			agentDBExists = true
			break
		}
	}
	if !agentDBExists {
		return report, fmt.Errorf("milvus database %s not found", cfg.DBName)
	}

	agentClient, err := cli.NewClient(ctx, cli.Config{
		Address: cfg.Addr,
		DBName:  cfg.DBName,
	})
	if err != nil {
		return report, err
	}
	defer agentClient.Close()

	collections, err := agentClient.ListCollections(ctx)
	if err != nil {
		return report, fmt.Errorf("failed to list collections: %w", err)
	}
	exists := false
	for _, collection := range collections {
		if collection.Name == cfg.CollectionName {
			exists = true
			break
		}
	}
	if !exists {
		return report, fmt.Errorf("milvus collection %s not found", cfg.CollectionName)
	}

	collection, err := agentClient.DescribeCollection(ctx, cfg.CollectionName)
	if err != nil {
		return report, err
	}
	if err := ValidateCollectionSchema(cfg, collection); err != nil {
		return report, err
	}
	report.SchemaOK = true

	stats, err := agentClient.GetCollectionStatistics(ctx, cfg.CollectionName)
	if err != nil {
		return report, err
	}
	if raw := strings.TrimSpace(stats["row_count"]); raw != "" {
		if count, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
			report.DocCount = count
		}
	}

	return report, nil
}

func ValidateCollectionSchema(cfg MilvusConfig, collection *entity.Collection) error {
	if collection == nil || collection.Schema == nil {
		return fmt.Errorf("collection schema is empty")
	}

	fields := make(map[string]*entity.Field)
	for _, field := range collection.Schema.Fields {
		if field != nil {
			fields[field.Name] = field
		}
	}

	expected := BuildMilvusFields(cfg)
	for _, want := range expected {
		got := fields[want.Name]
		if got == nil {
			return fmt.Errorf("milvus collection schema mismatch: missing field %s", want.Name)
		}
		if got.DataType != want.DataType {
			return fmt.Errorf("milvus collection schema mismatch: field %s type=%v want=%v", want.Name, got.DataType, want.DataType)
		}
		if want.PrimaryKey && !got.PrimaryKey {
			return fmt.Errorf("milvus collection schema mismatch: field %s is not primary key", want.Name)
		}
		for key, wantValue := range want.TypeParams {
			if got.TypeParams[key] != wantValue {
				return fmt.Errorf("milvus collection schema mismatch: field %s %s=%s want=%s", want.Name, key, got.TypeParams[key], wantValue)
			}
		}
	}
	return nil
}
