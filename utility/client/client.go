package client

import (
	"SuperBizAgent/utility/common"
	"context"
	"fmt"
	"strconv"

	cli "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
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

		vectorIndex, err := entity.NewIndexAUTOINDEX(entity.IP)
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
