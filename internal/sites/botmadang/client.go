package botmadang

import (
	"bufio"
	"bytes"
	"context"
	"d3k-agent/internal/core/domain"
	"d3k-agent/internal/core/ports"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

const DefaultBaseURL = "https://botmadang.org/api/v1"

// Client는 봇마당(Botmadang) 커뮤니티 API를 위한 어댑터입니다.
// ports.Site 인터페이스를 구현하며 인증, 데이터 매핑, API 통신을 담당합니다.
type Client struct {
	BaseURL    string
	APIKey     string
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
	return "botmadang"
}

func (c *Client) Initialize(ctx context.Context) error {
	// 1. Try to load API key from storage
	token, _ := c.Storage.LoadToken(c.Name())
	if token != "" {
		c.APIKey = token
		// Verify token validity
		if err := c.checkToken(ctx); err == nil {
			fmt.Printf("✅ [%s] Authenticated successfully.\n", c.Name())
			return nil
		}
		fmt.Printf("⚠️  [%s] Saved token is invalid or expired.\n", c.Name())
	}

	// 2. Start Interactive Registration Flow
	fmt.Printf("\n🚀 [%s] Starting New Agent Registration\n", c.Name())
	reader := bufio.NewReader(os.Stdin)

	// Ask for Name
	fmt.Print("Enter Bot Name (default: D3K_Bot): ")
	botName, _ := reader.ReadString('\n')
	botName = strings.TrimSpace(botName)
	if botName == "" {
		botName = "D3K_Bot"
	}

	// Register
	regResp, err := c.Register(botName, "An automated agent for Botmadang")
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	// Show Instructions
	fmt.Printf("\n=== 🛡️  Verification Required ===\n")
	fmt.Printf("1. Open this URL: %s\n", regResp.Agent.ClaimURL)
	fmt.Printf("2. Post a tweet containing this code: %s\n", regResp.Agent.VerificationCode)
	fmt.Printf("3. Copy the link to your tweet.\n")
	fmt.Println("=================================")

	// Ask for Tweet URL
	fmt.Print("\n🔗 Enter Tweet URL: ")
	tweetURL, _ := reader.ReadString('\n')
	tweetURL = strings.TrimSpace(tweetURL)

	if tweetURL == "" {
		return fmt.Errorf("tweet URL is required")
	}

	// Verify
	fmt.Print("Verifying... ")
	apiKey, err := c.Verify(regResp.Agent.VerificationCode, tweetURL)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("verification failed: %w", err)
	}
	fmt.Println("SUCCESS!")

	// Save
	c.APIKey = apiKey
	if err := c.Storage.SaveToken(c.Name(), apiKey); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	fmt.Printf("✅ [%s] Registration complete! API Key saved.\n", c.Name())
	return nil
}

// checkToken verifies if the current API Key is valid
func (c *Client) checkToken(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/agents/me", nil)
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid token, status: %d", resp.StatusCode)
	}
	return nil
}

// Register 새로운 에이전트 등록
func (c *Client) Register(name, description string) (*RegisterResponse, error) {
	reqBody, _ := json.Marshal(RegisterRequest{Name: name, Description: description})
	resp, err := c.HTTPClient.Post(c.BaseURL+"/agents/register", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("register failed with status: %d", resp.StatusCode)
	}

	var res RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Verify completes the registration process
func (c *Client) Verify(code, tweetURL string) (string, error) {
	reqBody, _ := json.Marshal(VerifyRequest{TweetURL: tweetURL})
	url := fmt.Sprintf("%s/claim/%s/verify", c.BaseURL, code)
	
	resp, err := c.HTTPClient.Post(url, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errRes struct{ Message string `json:"message"` }
		json.NewDecoder(resp.Body).Decode(&errRes)
		return "", fmt.Errorf("verify failed (%d): %s", resp.StatusCode, errRes.Message)
	}

	var res VerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	
	if !res.Success {
		return "", fmt.Errorf(res.Message)
	}

	return res.APIKey, nil
}

// GetRecentPosts implements ports.Site
func (c *Client) GetRecentPosts(ctx context.Context, limit int) ([]domain.Post, error) {
	// Use limit and context
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/posts?limit=%d", c.BaseURL, limit), nil)
	// API Key is optional for reading posts, but good to include if we have it
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Success bool      `json:"success"`
		Posts   []ApiPost `json:"posts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var corePosts []domain.Post
	for _, p := range data.Posts {
		corePosts = append(corePosts, domain.Post{
			ID:        p.ID,
			Title:     p.Title,
			Content:   p.Content,
			Author:    p.AuthorName,
			URL:       fmtURL(p.ID),
			Source:    "botmadang",
			CreatedAt: p.CreatedAt,
		})
	}
	return corePosts, nil
}

func (c *Client) GetNotifications(ctx context.Context, unreadOnly bool) ([]domain.Notification, error) {
	url := fmt.Sprintf("%s/notifications?limit=20", c.BaseURL)
	if unreadOnly {
		url += "&unread_only=true"
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch notifications: status %d", resp.StatusCode)
	}

	var data struct {
		Success       bool `json:"success"`
		Notifications []struct {
			ID             string `json:"id"`
			Type           string `json:"type"`
			ActorName      string `json:"actor_name"`
			PostID         string `json:"post_id"`
			PostTitle      string `json:"post_title"`
			CommentID      string `json:"comment_id"`
			ContentPreview string `json:"content_preview"`
			IsRead         bool   `json:"is_read"`
			CreatedAt      string `json:"created_at"` // Handle string time
		} `json:"notifications"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var notifs []domain.Notification
	for _, n := range data.Notifications {
		notifs = append(notifs, domain.Notification{
			ID:        n.ID,
			Type:      n.Type,
			Source:    "botmadang",
			ActorName: n.ActorName,
			PostID:    n.PostID,
			PostTitle: n.PostTitle,
			CommentID: n.CommentID,
			Content:   n.ContentPreview,
			IsRead:    n.IsRead,
			// Time parsing omitted for brevity, acceptable for now
		})
	}

	return notifs, nil
}

func (c *Client) CreatePost(ctx context.Context, post domain.Post) error {
	// TODO: Implement API call with Rate Limiting logic
	return nil
}

func (c *Client) CreateComment(ctx context.Context, postID string, content string) error {
	// TODO: Implement API call with Rate Limiting logic
	return nil
}

func (c *Client) ReplyToComment(ctx context.Context, postID, parentCommentID, content string) error {
	reqBody, _ := json.Marshal(map[string]string{
		"content":   content,
		"parent_id": parentCommentID,
	})

	url := fmt.Sprintf("%s/posts/%s/comments", c.BaseURL, postID)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var errRes struct{ Message string `json:"message"` }
		json.NewDecoder(resp.Body).Decode(&errRes)
		return fmt.Errorf("reply failed (%d): %s", resp.StatusCode, errRes.Message)
	}

	return nil
}

func (c *Client) MarkNotificationRead(ctx context.Context, id string) error {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"notification_ids": []string{id},
	})

	url := fmt.Sprintf("%s/notifications/read", c.BaseURL)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mark read failed: status %d", resp.StatusCode)
	}

	return nil
}

func fmtURL(id string) string {
	return "https://botmadang.org/post/" + id
}
