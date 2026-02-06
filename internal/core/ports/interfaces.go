package ports

import (
	"context"
	"d3k-agent/internal/core/domain"
)

type Site interface {
	Name() string
	Initialize(ctx context.Context) error
	GetRecentPosts(ctx context.Context, limit int) ([]domain.Post, error)
	GetNotifications(ctx context.Context, unreadOnly bool) ([]domain.Notification, error)
	CreatePost(ctx context.Context, post domain.Post) error
	CreateComment(ctx context.Context, postID string, content string) error
	ReplyToComment(ctx context.Context, postID, parentCommentID, content string) error
	MarkNotificationRead(ctx context.Context, id string) error
}

type Brain interface {
	GeneratePost(ctx context.Context, topic string) (string, error)
	GenerateReply(ctx context.Context, postContent string, commentContent string) (string, error)
	EvaluatePost(ctx context.Context, post domain.Post) (int, string, error)
	// SummarizeInsight generates a one-line lesson from a post.
	SummarizeInsight(ctx context.Context, post domain.Post) (string, error)
}

type Storage interface {
	SaveCursor(source string, cursor string) error
	LoadCursor(source string) (string, error)
	GetPostStats(source string) (int, string, int64, error)
	IncrementPostCount(source string, date string, timestamp int64) error
	GetCommentStats(source string) (int, string, error)
	IncrementCommentCount(source string, date string) error
	IsProactiveDone(source, postID string) (bool, error)
	MarkProactive(source, postID string) error
	
	// Memory/Insight System
	SaveInsight(ctx context.Context, insight domain.Insight) error
	GetRecentInsights(ctx context.Context, limit int) ([]domain.Insight, error)
}

type UserAction string

const (
	ActionApprove    UserAction = "approve"
	ActionRegenerate UserAction = "regenerate"
	ActionSkip       UserAction = "skip"
)

type Interaction interface {
	Confirm(ctx context.Context, title, body string) (UserAction, error)
}