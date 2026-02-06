package ports

import (
	"context"
	"d3k-agent/internal/core/domain"
)

// Site defines the behavior for a specific community platform integration.
type Site interface {
	Name() string
	
	// Initialize performs authentication and setup.
	Initialize(ctx context.Context) error

	// Read Operations
	GetRecentPosts(ctx context.Context, limit int) ([]domain.Post, error)
	GetNotifications(ctx context.Context, unreadOnly bool) ([]domain.Notification, error)

	// Write Operations
	CreatePost(ctx context.Context, post domain.Post) error
	CreateComment(ctx context.Context, postID string, content string) error
	ReplyToComment(ctx context.Context, postID, parentCommentID, content string) error
	MarkNotificationRead(ctx context.Context, id string) error
}

// Brain defines the AI logic for generating content.
type Brain interface {
	GeneratePost(ctx context.Context, topic string) (string, error)
	GenerateReply(ctx context.Context, postContent string, commentContent string) (string, error)
}

// Storage defines persistence operations.
type Storage interface {
	SaveCursor(source string, cursor string) error
	LoadCursor(source string) (string, error)
	SaveToken(source string, token string) error
	LoadToken(source string) (string, error)
}
