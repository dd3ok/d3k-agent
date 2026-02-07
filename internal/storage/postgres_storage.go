package storage

import (
	"context"
	"d3k-agent/internal/core/domain"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStorage struct {
	Pool *pgxpool.Pool
}

func NewPostgresStorage(ctx context.Context, connStr string) (*PostgresStorage, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %v", err)
	}

	s := &PostgresStorage{Pool: pool}
	if err := s.initSchema(ctx); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *PostgresStorage) initSchema(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS cursors (source TEXT PRIMARY KEY, cursor TEXT)`,
		`CREATE TABLE IF NOT EXISTS post_stats (source TEXT PRIMARY KEY, count INT, last_date TEXT, last_timestamp BIGINT)`,
		`CREATE TABLE IF NOT EXISTS comment_stats (source TEXT PRIMARY KEY, count INT, last_date TEXT)`,
		`CREATE TABLE IF NOT EXISTS proactive_log (source TEXT, post_id TEXT, PRIMARY KEY(source, post_id))`,
		`CREATE TABLE IF NOT EXISTS pending_actions (action_id TEXT PRIMARY KEY, created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS insights (
			id SERIAL PRIMARY KEY,
			post_id TEXT,
			source TEXT,
			topic TEXT,
			content TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, q := range queries {
		if _, err := s.Pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("failed to init schema: %v", err)
		}
	}
	return nil
}

func (s *PostgresStorage) SaveCursor(source, cursor string) error {
	_, err := s.Pool.Exec(context.Background(), 
		"INSERT INTO cursors (source, cursor) VALUES ($1, $2) ON CONFLICT (source) DO UPDATE SET cursor = $2", 
		source, cursor)
	return err
}

func (s *PostgresStorage) LoadCursor(source string) (string, error) {
	var cursor string
	err := s.Pool.QueryRow(context.Background(), "SELECT cursor FROM cursors WHERE source = $1", source).Scan(&cursor)
	if err != nil { return "", nil }
	return cursor, nil
}

func (s *PostgresStorage) GetPostStats(source string) (int, string, int64, error) {
	var count int; var lastDate string; var lastTs int64
	err := s.Pool.QueryRow(context.Background(), "SELECT count, last_date, last_timestamp FROM post_stats WHERE source = $1", source).Scan(&count, &lastDate, &lastTs)
	if err != nil { return 0, "", 0, nil }
	return count, lastDate, lastTs, nil
}

func (s *PostgresStorage) IncrementPostCount(source string, date string, timestamp int64) error {
	_, err := s.Pool.Exec(context.Background(), 
		`INSERT INTO post_stats (source, count, last_date, last_timestamp) VALUES ($1, 1, $2, $3) 
		 ON CONFLICT (source) DO UPDATE SET 
		 count = CASE WHEN post_stats.last_date = $2 THEN post_stats.count + 1 ELSE 1 END,
		 last_date = $2, last_timestamp = $3`, 
		source, date, timestamp)
	return err
}

func (s *PostgresStorage) GetCommentStats(source string) (int, string, error) {
	var count int; var lastDate string
	err := s.Pool.QueryRow(context.Background(), "SELECT count, last_date FROM comment_stats WHERE source = $1", source).Scan(&count, &lastDate)
	if err != nil { return 0, "", nil }
	return count, lastDate, nil
}

func (s *PostgresStorage) IncrementCommentCount(source string, date string) error {
	_, err := s.Pool.Exec(context.Background(), 
		`INSERT INTO comment_stats (source, count, last_date) VALUES ($1, 1, $2) 
		 ON CONFLICT (source) DO UPDATE SET 
		 count = CASE WHEN comment_stats.last_date = $2 THEN comment_stats.count + 1 ELSE 1 END,
		 last_date = $2`, 
		source, date)
	return err
}

func (s *PostgresStorage) IsProactiveDone(source, postID string) (bool, error) {
	var exists bool
	err := s.Pool.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM proactive_log WHERE source=$1 AND post_id=$2)", source, postID).Scan(&exists)
	return exists, err
}

func (s *PostgresStorage) MarkProactive(source, postID string) error {
	_, err := s.Pool.Exec(context.Background(), "INSERT INTO proactive_log (source, post_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", source, postID)
	return err
}

func (s *PostgresStorage) IsPending(actionID string) (bool, error) {
	var exists bool
	err := s.Pool.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM pending_actions WHERE action_id=$1)", actionID).Scan(&exists)
	return exists, err
}

func (s *PostgresStorage) SetPending(actionID string) error {
	_, err := s.Pool.Exec(context.Background(), "INSERT INTO pending_actions (action_id) VALUES ($1) ON CONFLICT DO NOTHING", actionID)
	return err
}

func (s *PostgresStorage) ClearPending(actionID string) error {
	_, err := s.Pool.Exec(context.Background(), "DELETE FROM pending_actions WHERE action_id = $1", actionID)
	return err
}

func (s *PostgresStorage) SaveInsight(ctx context.Context, i domain.Insight) error {
	_, err := s.Pool.Exec(ctx, "INSERT INTO insights (post_id, source, topic, content) VALUES ($1, $2, $3, $4)", i.PostID, i.Source, i.Topic, i.Content)
	return err
}

func (s *PostgresStorage) GetRecentInsights(ctx context.Context, limit int) ([]domain.Insight, error) {
	rows, err := s.Pool.Query(ctx, "SELECT id, post_id, source, topic, content, created_at FROM insights ORDER BY created_at DESC LIMIT $1", limit)
	if err != nil { return nil, err }
	defer rows.Close()

	var res []domain.Insight
	for rows.Next() {
		var i domain.Insight
		rows.Scan(&i.ID, &i.PostID, &i.Source, &i.Topic, &i.Content, &i.CreatedAt)
		res = append(res, i)
	}
	return res, nil
}
