package client

import (
	"SuperBizAgent/utility/common"
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
	milvusClientsMu sync.Mutex
	milvusClients   []cli.Client
)

func NewMilvusClient(ctx context.Context) (cli.Client, error) {
	addr := common.GetMilvusAddr(ctx)

	defaultClient, err := cli.NewClient(ctx, cli.Config{
		Address: addr,
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
		if db.Name == common.MilvusDBName {
			agentDBExists = true
			break
		}
	}
	if !agentDBExists {
		err = defaultClient.CreateDatabase(ctx, common.MilvusDBName)
		if err != nil {
			return nil, fmt.Errorf("failed to create agent database: %w", err)
		}
	}

	agentClient, err := cli.NewClient(ctx, cli.Config{
		Address: addr,
		DBName:  common.MilvusDBName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to agent database: %w", err)
	}
	registerMilvusClient(agentClient)

	collections, err := agentClient.ListCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}

	bizCollectionExists := false
	for _, collection := range collections {
		if collection.Name == common.MilvusCollectionName {
			bizCollectionExists = true
			break
		}
	}

	if !bizCollectionExists {
		collSchema := &entity.Schema{
			CollectionName: common.MilvusCollectionName,
			Description:    "Business knowledge collection",
			Fields:         BuildMilvusFields(ctx),
		}

		err = agentClient.CreateCollection(ctx, collSchema, entity.DefaultShardNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to create biz collection: %w", err)
		}

		vectorIndex, err := buildMilvusVectorIndex(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create vector index: %w", err)
		}
		err = agentClient.CreateIndex(ctx, common.MilvusCollectionName, "vector", vectorIndex, false)
		if err != nil {
			return nil, fmt.Errorf("failed to create vector index: %w", err)
		}
	}

	err = agentClient.LoadCollection(ctx, common.MilvusCollectionName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to load biz collection: %w", err)
	}

	return agentClient, nil
}

func CloseAllMilvusClients() error {
	milvusClientsMu.Lock()
	clients := milvusClients
	milvusClients = nil
	milvusClientsMu.Unlock()

	var errs []string
	for _, c := range clients {
		if c == nil {
			continue
		}
		if err := c.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("milvus close failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

func registerMilvusClient(c cli.Client) {
	if c == nil {
		return
	}
	milvusClientsMu.Lock()
	milvusClients = append(milvusClients, c)
	milvusClientsMu.Unlock()
}

func buildMilvusVectorIndex(ctx context.Context) (entity.Index, error) {
	metricType, err := resolveMilvusMetricType(ctx)
	if err != nil {
		return nil, err
	}

	switch strings.ToUpper(strings.TrimSpace(common.GetMilvusIndexType(ctx))) {
	case "HNSW":
		m := common.GetMilvusHNSWM(ctx)
		efConstruction := common.GetMilvusHNSWEfConstruction(ctx)
		g.Log().Infof(ctx, "creating Milvus HNSW index, metric=%s, m=%d, efConstruction=%d", string(metricType), m, efConstruction)
		return entity.NewIndexHNSW(metricType, m, efConstruction)
	case "AUTOINDEX", "AUTO":
		g.Log().Infof(ctx, "creating Milvus AUTOINDEX, metric=%s", string(metricType))
		return entity.NewIndexAUTOINDEX(metricType)
	default:
		return nil, fmt.Errorf("unsupported milvus.index_type: %s", common.GetMilvusIndexType(ctx))
	}
}

func resolveMilvusMetricType(ctx context.Context) (entity.MetricType, error) {
	switch strings.ToUpper(strings.TrimSpace(common.GetMilvusMetricType(ctx))) {
	case "IP":
		return entity.IP, nil
	case "L2":
		return entity.L2, nil
	case "COSINE":
		return entity.COSINE, nil
	default:
		return "", fmt.Errorf("unsupported milvus.metric_type: %s", common.GetMilvusMetricType(ctx))
	}
}

func BuildMilvusFields(ctx context.Context) []*entity.Field {
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
				"dim": strconv.Itoa(common.GetVectorDimension(ctx)),
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
