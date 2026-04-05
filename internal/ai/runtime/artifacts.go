package runtime

import (
	"context"
	"fmt"
	"sync"

	"SuperBizAgent/internal/ai/protocol"
)

type ArtifactStore interface {
	Put(ctx context.Context, artifact *protocol.Artifact) (*protocol.ArtifactRef, error)
	Get(ctx context.Context, ref *protocol.ArtifactRef) (*protocol.Artifact, error)
}

type InMemoryArtifactStore struct {
	mu        sync.RWMutex
	artifacts map[string]*protocol.Artifact
}

func NewInMemoryArtifactStore() *InMemoryArtifactStore {
	return &InMemoryArtifactStore{
		artifacts: make(map[string]*protocol.Artifact),
	}
}

func (s *InMemoryArtifactStore) Put(_ context.Context, artifact *protocol.Artifact) (*protocol.ArtifactRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *artifact
	s.artifacts[artifact.Ref.ID] = &cp
	ref := artifact.Ref
	return &ref, nil
}

func (s *InMemoryArtifactStore) Get(_ context.Context, ref *protocol.ArtifactRef) (*protocol.Artifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	artifact, ok := s.artifacts[ref.ID]
	if !ok {
		return nil, fmt.Errorf("artifact %q not found", ref.ID)
	}
	cp := *artifact
	return &cp, nil
}
