package rag

import (
	"sync"
)

var (
	sharedBM25Mu    sync.RWMutex
	sharedBM25Index *BM25Index
)

func SharedBM25Index() *BM25Index {
	sharedBM25Mu.RLock()
	idx := sharedBM25Index
	sharedBM25Mu.RUnlock()
	if idx != nil {
		return idx
	}

	sharedBM25Mu.Lock()
	defer sharedBM25Mu.Unlock()
	if sharedBM25Index == nil {
		sharedBM25Index = NewBM25Index()
	}
	return sharedBM25Index
}

func SetSharedBM25Index(idx *BM25Index) {
	sharedBM25Mu.Lock()
	sharedBM25Index = idx
	sharedBM25Mu.Unlock()
}

func ResetSharedBM25Index() {
	sharedBM25Mu.Lock()
	sharedBM25Index = nil
	sharedBM25Mu.Unlock()
}
