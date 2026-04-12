package rag

import (
	"sync"
)

var (
	sharedBM25Once  sync.Once
	sharedBM25Index *BM25Index
)

func SharedBM25Index() *BM25Index {
	sharedBM25Once.Do(func() {
		sharedBM25Index = NewBM25Index()
	})
	return sharedBM25Index
}

func SetSharedBM25Index(idx *BM25Index) {
	sharedBM25Once.Do(func() {})
	sharedBM25Index = idx
}

func ResetSharedBM25Index() {
	sharedBM25Once = sync.Once{}
	sharedBM25Index = nil
}
