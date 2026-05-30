package milvus

import (
	"SuperBizAgent/utility/common"
	"context"
)

const (
	DefaultDBName         = "agent"
	DefaultCollectionName = "opscaption_knowledge_v2"
)

type MilvusConfig struct {
	Addr               string
	DBName             string
	CollectionName     string
	IndexType          string
	MetricType         string
	HNSWM              int
	HNSWEfConstruction int
	VectorDimension    int
}

func MilvusConfigFromContext(ctx context.Context) MilvusConfig {
	return MilvusConfig{
		Addr:               common.GetMilvusAddr(ctx),
		DBName:             DefaultDBName,
		CollectionName:     common.GetMilvusCollectionName(ctx),
		IndexType:          common.GetMilvusIndexType(ctx),
		MetricType:         common.GetMilvusMetricType(ctx),
		HNSWM:              common.GetMilvusHNSWM(ctx),
		HNSWEfConstruction: common.GetMilvusHNSWEfConstruction(ctx),
		VectorDimension:    common.GetVectorDimension(ctx),
	}
}
