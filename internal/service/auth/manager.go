package auth

import (
	"sync"
)

// ProviderManager 简单注册/获取 provider
type ProviderManager struct {
	providers map[string]AuthProvider
	mu        sync.RWMutex
}

func NewProviderManager() *ProviderManager {
	return &ProviderManager{
		providers: make(map[string]AuthProvider),
	}
}

func (m *ProviderManager) Register(name string, p AuthProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[name] = p
}

func (m *ProviderManager) Get(name string) (AuthProvider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[name]
	return p, ok
}
