package mem

import (
	"context"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/frame/g"
)

var (
	globalSessionStore     SessionMemoryStore
	globalSessionStoreOnce sync.Once
	globalLTMStore         LongTermMemoryBackend
	globalLTMStoreOnce     sync.Once
)

func GetSessionStore() SessionMemoryStore {
	globalSessionStoreOnce.Do(func() {
		if useRedisBackend() {
			globalSessionStore = NewRedisSessionStore()
		} else {
			globalSessionStore = NewInMemorySessionStore()
		}
	})
	return globalSessionStore
}

func GetLongTermStore() LongTermMemoryBackend {
	globalLTMStoreOnce.Do(func() {
		if useRedisBackend() {
			globalLTMStore = NewRedisLongTermStore()
		} else {
			globalLTMStore = NewInMemoryLongTermStore()
		}
	})
	return globalLTMStore
}

func useRedisBackend() bool {
	v, err := g.Cfg().Get(context.Background(), "memory.backend")
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(v.String()), "redis")
}

func SetSessionStoreForTesting(store SessionMemoryStore) {
	globalSessionStoreOnce.Do(func() {})
	globalSessionStore = store
}

func SetLongTermStoreForTesting(store LongTermMemoryBackend) {
	globalLTMStoreOnce.Do(func() {})
	globalLTMStore = store
}
