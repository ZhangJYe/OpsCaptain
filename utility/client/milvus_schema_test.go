package client

import (
	"context"
	"strings"
	"testing"

	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

func TestValidateMilvusCollectionSchemaMatchesExpected(t *testing.T) {
	ctx := context.Background()
	collection := &entity.Collection{
		Schema: &entity.Schema{
			CollectionName: "opscaption_knowledge_v2",
			Fields:         BuildMilvusFields(ctx),
		},
	}

	if err := ValidateMilvusCollectionSchema(ctx, collection); err != nil {
		t.Fatalf("expected schema to match: %v", err)
	}
}

func TestValidateMilvusCollectionSchemaRejectsLegacySchema(t *testing.T) {
	ctx := context.Background()
	collection := &entity.Collection{
		Schema: &entity.Schema{
			CollectionName: "biz",
			Fields: []*entity.Field{
				{
					Name:       "id",
					DataType:   entity.FieldTypeVarChar,
					PrimaryKey: true,
					TypeParams: map[string]string{"max_length": "256"},
				},
				{
					Name:       "vector",
					DataType:   entity.FieldTypeFloatVector,
					TypeParams: map[string]string{"dim": "2048"},
				},
			},
		},
	}

	err := ValidateMilvusCollectionSchema(ctx, collection)
	if err == nil {
		t.Fatal("expected schema mismatch")
	}
	if !strings.Contains(err.Error(), "missing field content") {
		t.Fatalf("expected missing content error, got %v", err)
	}
}
