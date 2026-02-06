package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"d3k-agent/internal/core/ports"
)

// JSONStorage는 간단한 로컬 파일 기반의 영속성 메커니즘을 제공합니다.
type JSONStorage struct {
	FilePath string
	mu       sync.RWMutex
	Data     StorageData
}

type StorageData struct {
	Cursors           map[string]string `json:"cursors"`
	DailyPostCount    map[string]int    `json:"daily_post_count"`
	LastPostDate      map[string]string `json:"last_post_date"`
	LastPostTimestamp map[string]int64  `json:"last_post_timestamp"`
	DailyCommentCount map[string]int    `json:"daily_comment_count"`
	LastCommentDate   map[string]string `json:"last_comment_date"`
}

func NewJSONStorage(filePath string) (*JSONStorage, error) {
	s := &JSONStorage{
		FilePath: filePath,
		Data: StorageData{
			Cursors:           make(map[string]string),
			DailyPostCount:    make(map[string]int),
			LastPostDate:      make(map[string]string),
			LastPostTimestamp: make(map[string]int64),
			DailyCommentCount: make(map[string]int),
			LastCommentDate:   make(map[string]string),
		},
	}
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil { return nil, err }
	
	// 초기 로드
	if err := s.loadFromFile(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

var _ ports.Storage = (*JSONStorage)(nil)

// 내부 헬퍼 (락 없이 호출됨)
func (s *JSONStorage) loadFromFile() error {
	file, err := os.ReadFile(s.FilePath)
	if err != nil { return err }
	return json.Unmarshal(file, &s.Data)
}

// 내부 헬퍼 (락 없이 호출됨)
func (s *JSONStorage) saveToFile() error {
	data, err := json.MarshalIndent(s.Data, "", "  ")
	if err != nil { return err }
	return os.WriteFile(s.FilePath, data, 0644)
}

func (s *JSONStorage) SaveCursor(source string, cursor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Data.Cursors[source] = cursor
	return s.saveToFile()
}

func (s *JSONStorage) LoadCursor(source string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// 실행 중인 메모리 데이터를 우선 반환하며, 
	// 필요시 파일에서 다시 읽어오는 로직은 생략(성능 및 단순성)
	return s.Data.Cursors[source], nil
}

func (s *JSONStorage) GetPostStats(source string) (int, string, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Data.DailyPostCount[source], s.Data.LastPostDate[source], s.Data.LastPostTimestamp[source], nil
}

func (s *JSONStorage) IncrementPostCount(source string, date string, timestamp int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Data.LastPostDate[source] != date {
		s.Data.DailyPostCount[source] = 1
		s.Data.LastPostDate[source] = date
	} else {
		s.Data.DailyPostCount[source]++
	}
	s.Data.LastPostTimestamp[source] = timestamp
	return s.saveToFile()
}

func (s *JSONStorage) GetCommentStats(source string) (int, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Data.DailyCommentCount[source], s.Data.LastCommentDate[source], nil
}

func (s *JSONStorage) IncrementCommentCount(source string, date string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Data.LastCommentDate[source] != date {
		s.Data.DailyCommentCount[source] = 1
		s.Data.LastCommentDate[source] = date
	} else {
		s.Data.DailyCommentCount[source]++
	}
	return s.saveToFile()
}
