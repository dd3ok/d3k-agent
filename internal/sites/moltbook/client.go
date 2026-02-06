package moltbook

import (
	"context"
	"d3k-agent/internal/core/domain"
	"d3k-agent/internal/core/ports"
	"net/http"
)

const DefaultBaseURL = "https://www.moltbook.com/api"

// Client는 몰트북(Moltbook) 플랫폼을 위한 어댑터입니다.
// 현재는 멀티 사이트 아키텍처의 확장성을 보여주기 위한 예시 구현체(Placeholder) 역할을 합니다.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Storage    ports.Storage
}

func NewClient(storage ports.Storage) *Client {
	return &Client{
		BaseURL:    DefaultBaseURL,
		HTTPClient: &http.Client{},
		Storage:    storage,
	}
}

// Ensure Client implements Site interface
var _ ports.Site = (*Client)(nil)

func (c *Client) Name() string {
	return "moltbook"
}

func (c *Client) Initialize(ctx context.Context) error {
	return nil
}

func (c *Client) GetRecentPosts(ctx context.Context, limit int) ([]domain.Post, error) {
	return []domain.Post{}, nil
}

func (c *Client) GetNotifications(ctx context.Context, unreadOnly bool) ([]domain.Notification, error) {
	return []domain.Notification{}, nil
}

func (c *Client) CreatePost(ctx context.Context, post domain.Post) error {
	return nil
}

func (c *Client) CreateComment(ctx context.Context, postID string, content string) error {
	return nil
}

func (c *Client) ReplyToComment(ctx context.Context, postID, parentCommentID, content string) error {
	return nil
}

func (c *Client) MarkNotificationRead(ctx context.Context, id string) error {
	return nil
}
