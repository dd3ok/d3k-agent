package moltbook

import (
	"bufio"
	"bytes"
	"context"
	"d3k-agent/internal/core/domain"
	"d3k-agent/internal/core/ports"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const DefaultBaseURL = "https://www.moltbook.com/api/v1"

// Client는 Moltbook 커뮤니티 API를 위한 어댑터입니다.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Storage    ports.Storage
}

func NewClient(storage ports.Storage) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		Storage: storage,
	}
}

var _ ports.Site = (*Client)(nil)

func (c *Client) Name() string {
	return "moltbook"
}

func (c *Client) Initialize(ctx context.Context) error {
	// 1. .env 확인
	token := os.Getenv("MOLTBOOK_API_KEY")
	if token != "" {
		c.APIKey = token
		fmt.Printf("✅ [%s] Authenticated via .env\n", c.Name())
		return nil
	}

	// 2. 등록 절차
	fmt.Printf("\n🚀 [%s] Starting New Registration...\n", c.Name())
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Moltbook 봇 이름을 입력하세요: ")
	botName, _ := reader.ReadString('\n')
	botName = strings.TrimSpace(botName)
	if botName == "" { botName = "d3k_bot" }

	regResp, err := c.Register(botName, "지적인 대화와 영감을 나누는 AI 에이전트 d3k입니다.")
	if err != nil { return err }

	fmt.Printf("\n=== 🛡️  인증 필요 (Moltbook) ===\n")
	fmt.Printf("1. URL 접속: %s\n", regResp.Agent.ClaimURL)
	fmt.Printf("2. 발급된 API 키를 안전하게 보관하세요.\n")
	fmt.Println("=================================")

	fmt.Printf("\n🔑 발급된 API 키: %s\n", regResp.Agent.APIKey)
	fmt.Println("=========================================================")
	fmt.Println("⚠️  중요: 위 키를 '.env' 파일의 MOLTBOOK_API_KEY 항목에 입력하세요.")
	fmt.Println("=========================================================")

	os.Exit(0)
	return nil
}

func (c *Client) Register(name, description string) (*RegisterResponse, error) {
	reqBody, _ := json.Marshal(RegisterRequest{Name: name, Description: description})
	resp, err := c.HTTPClient.Post(c.BaseURL+"/agents/register", "application/json", bytes.NewBuffer(reqBody))
	if err != nil { return nil, err }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error: %s", string(body))
	}
	var res RegisterResponse
	json.NewDecoder(resp.Body).Decode(&res)
	return &res, nil
}

func (c *Client) GetRecentPosts(ctx context.Context, limit int) ([]domain.Post, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/posts?limit=%d", c.BaseURL, limit), nil)
	if c.APIKey != "" { req.Header.Set("Authorization", "Bearer "+c.APIKey) }
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK { return nil, fmt.Errorf("fail: %d", resp.StatusCode) }
	var data struct { Success bool `json:"success"`; Posts []ApiPost `json:"posts"` }
	json.NewDecoder(resp.Body).Decode(&data)

	var corePosts []domain.Post
	for _, p := range data.Posts {
		corePosts = append(corePosts, domain.Post{ID: p.ID, Title: p.Title, Content: p.Content, Author: p.AuthorName, URL: "https://www.moltbook.com/post/" + p.ID, Source: "moltbook", CreatedAt: p.CreatedAt})
	}
	return corePosts, nil
}

func (c *Client) GetNotifications(ctx context.Context, unreadOnly bool) ([]domain.Notification, error) {
	// Moltbook의 알림 API 주소가 봇마당과 같다고 가정 (표준 준수)
	req, _ := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/notifications?unread_only=true", nil)
	if c.APIKey != "" { req.Header.Set("Authorization", "Bearer "+c.APIKey) }
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK { return nil, fmt.Errorf("fail: %d", resp.StatusCode) }
	var data struct { Success bool `json:"success"`; Notifications []struct { ID, Type, ActorName, PostID, PostTitle, CommentID, ContentPreview string; IsRead bool } `json:"notifications"` }
	json.NewDecoder(resp.Body).Decode(&data)

	var notifs []domain.Notification
	for _, n := range data.Notifications {
		notifs = append(notifs, domain.Notification{ID: n.ID, Type: n.Type, Source: "moltbook", ActorName: n.ActorName, PostID: n.PostID, PostTitle: n.PostTitle, CommentID: n.CommentID, Content: n.ContentPreview, IsRead: n.IsRead})
	}
	return notifs, nil
}

func (c *Client) CreatePost(ctx context.Context, post domain.Post) error {
	reqBody, _ := json.Marshal(map[string]string{"title": post.Title, "content": post.Content, "submadang": "general"})
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/posts", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" { req.Header.Set("Authorization", "Bearer "+c.APIKey) }
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	return nil
}

func (c *Client) CreateComment(ctx context.Context, postID string, content string) error {
	reqBody, _ := json.Marshal(map[string]string{"content": content})
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/posts/"+postID+"/comments", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" { req.Header.Set("Authorization", "Bearer "+c.APIKey) }
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	return nil
}

func (c *Client) ReplyToComment(ctx context.Context, postID, parentCommentID, content string) error {
	reqBody, _ := json.Marshal(map[string]string{"content": content, "parent_id": parentCommentID})
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/posts/"+postID+"/comments", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" { req.Header.Set("Authorization", "Bearer "+c.APIKey) }
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	return nil
}

func (c *Client) MarkNotificationRead(ctx context.Context, id string) error {
	reqBody, _ := json.Marshal(map[string]interface{}{"notification_ids": []string{id}})
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/notifications/read", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" { req.Header.Set("Authorization", "Bearer "+c.APIKey) }
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	return nil
}