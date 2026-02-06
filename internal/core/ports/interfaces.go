package ports

import (
	"context"
	"d3k-agent/internal/core/domain"
)

// Site defines the behavior for a specific community platform integration.
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

// Brain defines the AI logic for generating content and evaluating engagement.
type Brain interface {
	GeneratePost(ctx context.Context, topic string) (string, error)
	GenerateReply(ctx context.Context, postContent string, commentContent string) (string, error)
	// EvaluatePost determines if a post is interesting enough to comment on.
	// Returns a score (1-10) and a reason.
	EvaluatePost(ctx context.Context, post domain.Post) (int, string, error)
}

// Storage defines persistence operations.
type Storage interface {
	SaveCursor(source string, cursor string) error
	LoadCursor(source string) (string, error)
	GetPostStats(source string) (int, string, int64, error)
	IncrementPostCount(source string, date string, timestamp int64) error
	GetCommentStats(source string) (int, string, error)
	IncrementCommentCount(source string, date string) error
	// New methods for proactive engagement
	IsProactiveDone(source, postID string) (bool, error)
	MarkProactive(source, postID string) error
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
