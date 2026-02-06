package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"d3k-agent/internal/core/ports"
)

// JSONStorage는 간단한 로컬 파일 기반의 영속성 메커니즘을 제공합니다.
// ports.Storage 인터페이스를 구현하여 API 토큰과 커서 위치(마지막으로 읽은 알림 ID)를 JSON 파일에 저장합니다.
// RWMutex를 사용하여 스레드 안전(Thread-safe)하게 동작합니다.
type JSONStorage struct {
	FilePath string
	mu       sync.RWMutex
	Data     StorageData
}

type StorageData struct {
	Tokens  map[string]string `json:"tokens"`  // Site Name -> API Key
	Cursors map[string]string `json:"cursors"` // Site Name -> Last Cursor/ID
}

// NewJSONStorage creates a new storage instance.
func NewJSONStorage(filePath string) (*JSONStorage, error) {
	s := &JSONStorage{
		FilePath: filePath,
		Data: StorageData{
			Tokens:  make(map[string]string),
			Cursors: make(map[string]string),
		},
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// Try loading existing data
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return s, nil
}

// Ensure implementation
var _ ports.Storage = (*JSONStorage)(nil)

func (s *JSONStorage) load() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, err := os.ReadFile(s.FilePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(file, &s.Data)
}

func (s *JSONStorage) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(s.Data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.FilePath, data, 0644)
}

func (s *JSONStorage) SaveToken(source string, token string) error {
	s.Data.Tokens[source] = token
	return s.save()
}

func (s *JSONStorage) LoadToken(source string) (string, error) {
	if err := s.load(); err != nil {
		// If file doesn't exist yet, return empty
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Data.Tokens[source], nil
}

func (s *JSONStorage) SaveCursor(source string, cursor string) error {
	s.Data.Cursors[source] = cursor
	return s.save()
}

func (s *JSONStorage) LoadCursor(source string) (string, error) {
	if err := s.load(); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Data.Cursors[source], nil
}
