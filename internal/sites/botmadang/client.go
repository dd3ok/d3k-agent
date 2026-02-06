package botmadang

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
	"sync"
	"time"
)

const DefaultBaseURL = "https://botmadang.org/api/v1"

// Client는 봇마당(Botmadang) 커뮤니티 API를 위한 어댑터입니다.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Storage    ports.Storage

	// 속도 제한 관리를 위한 필드
	mu             sync.Mutex
	lastWriteTime  time.Time
	lastPostTime   time.Time
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
	return "botmadang"
}

// enforceRateLimit은 봇마당 정책에 따라 쓰기 작업 간의 최소 간격을 강제합니다.
// isPost가 true면 3분, false(댓글)면 10초를 기다립니다.
func (c *Client) enforceRateLimit(isPost bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var waitDuration time.Duration
	var lastTime time.Time

	if isPost {
		waitDuration = 3 * time.Minute
		lastTime = c.lastPostTime
	} else {
		waitDuration = 11 * time.Second // 정책은 10초지만 안전하게 11초
		lastTime = c.lastWriteTime
	}

	elapsed := time.Since(lastTime)
	if elapsed < waitDuration {
		sleepTime := waitDuration - elapsed
		fmt.Printf("⏳ [%s] 정책 준수를 위해 %v 동안 대기합니다...\n", c.Name(), sleepTime.Round(time.Second))
		time.Sleep(sleepTime)
	}

	now := time.Now()
	c.lastWriteTime = now
	if isPost {
		c.lastPostTime = now
	}
}

func (c *Client) Initialize(ctx context.Context) error {
	envToken := os.Getenv("BOTMADANG_API_KEY")
	if envToken != "" {
		c.APIKey = envToken
		if err := c.checkToken(ctx); err == nil {
			fmt.Printf("✅ [%s] .env 파일을 통해 인증되었습니다.\n", c.Name())
			return nil
		}
		fmt.Printf("⚠️  [%s] API Key in .env is invalid.\n", c.Name())
	}

	fmt.Printf("\n🚀 [%s] Starting New Registration...\n", c.Name())
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("봇 이름을 입력하세요: ")
	botName, _ := reader.ReadString('\n')
	botName = strings.TrimSpace(botName)
	if botName == "" { botName = "D3K_Bot" }

	regResp, err := c.Register(botName, "기술/금융/일상에 관한 이야기를 해보고 싶어요. 우리의 대화가 생각의 확장, 영감을 얻는데 도움이 되면 좋겠습니다.")
	if err != nil { return err }

	fmt.Printf("\n=== 🛡️  인증 필요 ===\n1. URL: %s\n2. 코드: %s\n=================================\n", regResp.Agent.ClaimURL, regResp.Agent.VerificationCode)
	fmt.Print("\n🔗 트윗 URL 입력: ")
	tweetURL, _ := reader.ReadString('\n')
	tweetURL = strings.TrimSpace(tweetURL)

	apiKey, err := c.Verify(regResp.Agent.VerificationCode, tweetURL)
	if err != nil { return err }

	fmt.Printf("\n✨ 인증 성공! 발급된 API 키:\n\n%s\n\n", apiKey)
	fmt.Println("=========================================================\n⚠️  중요: 위 키를 '.env' 파일의 BOTMADANG_API_KEY 항목에 입력하세요.\n=========================================================")
	os.Exit(0)
	return nil
}

func (c *Client) checkToken(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/agents/me", nil)
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { return fmt.Errorf("invalid token") }
	return nil
}

func (c *Client) Register(name, description string) (*RegisterResponse, error) {
	reqBody, _ := json.Marshal(RegisterRequest{Name: name, Description: description})
	resp, err := c.HTTPClient.Post(c.BaseURL+"/agents/register", "application/json", bytes.NewBuffer(reqBody))
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var res RegisterResponse
	json.NewDecoder(resp.Body).Decode(&res)
	return &res, nil
}

func (c *Client) Verify(code, tweetURL string) (string, error) {
	reqBody, _ := json.Marshal(VerifyRequest{TweetURL: tweetURL})
	resp, err := c.HTTPClient.Post(fmt.Sprintf("%s/claim/%s/verify", c.BaseURL, code), "application/json", bytes.NewBuffer(reqBody))
	if err != nil { return "", err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { return "", fmt.Errorf("verify failed: %d", resp.StatusCode) }
	var res VerifyResponse
	json.NewDecoder(resp.Body).Decode(&res)
	if !res.Success { return "", fmt.Errorf(res.Message) }
	return res.APIKey, nil
}

func (c *Client) GetRecentPosts(ctx context.Context, limit int) ([]domain.Post, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/posts?limit=%d", c.BaseURL, limit), nil)
	if c.APIKey != "" { req.Header.Set("Authorization", "Bearer "+c.APIKey) }
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { return nil, fmt.Errorf("fetch posts failed: %d", resp.StatusCode) }
	var data struct { Success bool `json:"success"`; Posts []ApiPost `json:"posts"` }
	json.NewDecoder(resp.Body).Decode(&data)
	var corePosts []domain.Post
	for _, p := range data.Posts {
		corePosts = append(corePosts, domain.Post{ID: p.ID, Title: p.Title, Content: p.Content, Author: p.AuthorName, URL: "https://botmadang.org/post/" + p.ID, Source: "botmadang", CreatedAt: p.CreatedAt})
	}
	return corePosts, nil
}

func (c *Client) GetNotifications(ctx context.Context, unreadOnly bool) ([]domain.Notification, error) {
	url := fmt.Sprintf("%s/notifications?limit=20", c.BaseURL)
	if unreadOnly { url += "&unread_only=true" }
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	if c.APIKey != "" { req.Header.Set("Authorization", "Bearer "+c.APIKey) }
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { return nil, fmt.Errorf("fetch notifs failed: %d", resp.StatusCode) }
	var data struct { Success bool `json:"success"`; Notifications []struct { ID, Type, ActorName, PostID, PostTitle, CommentID, ContentPreview string; IsRead bool } `json:"notifications"` }
	json.NewDecoder(resp.Body).Decode(&data)
	var notifs []domain.Notification
	for _, n := range data.Notifications {
		notifs = append(notifs, domain.Notification{ID: n.ID, Type: n.Type, Source: "botmadang", ActorName: n.ActorName, PostID: n.PostID, PostTitle: n.PostTitle, CommentID: n.CommentID, Content: n.ContentPreview, IsRead: n.IsRead})
	}
	return notifs, nil
}

func (c *Client) CreatePost(ctx context.Context, post domain.Post) error {
	c.enforceRateLimit(true) // 3분 대기 강제

	var payload struct { Title, Content, Submadang string }
	if err := json.Unmarshal([]byte(post.Content), &payload); err != nil { payload.Title = post.Title; payload.Content = post.Content }
	if payload.Submadang == "" { payload.Submadang = "general" }
	reqBody, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/posts", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" { req.Header.Set("Authorization", "Bearer "+c.APIKey) }
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated { return fmt.Errorf("post failed: %d", resp.StatusCode) }
	return nil
}

func (c *Client) CreateComment(ctx context.Context, postID string, content string) error {
	c.enforceRateLimit(false) // 10초 대기 강제

	reqBody, _ := json.Marshal(map[string]string{"content": content})
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/posts/"+postID+"/comments", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" { req.Header.Set("Authorization", "Bearer "+c.APIKey) }
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated { return fmt.Errorf("comment failed: %d", resp.StatusCode) }
	return nil
}

func (c *Client) ReplyToComment(ctx context.Context, postID, parentCommentID, content string) error {
	c.enforceRateLimit(false) // 10초 대기 강제

	reqBody, _ := json.Marshal(map[string]string{"content": content, "parent_id": parentCommentID})
	req, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/posts/"+postID+"/comments", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" { req.Header.Set("Authorization", "Bearer "+c.APIKey) }
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated { return fmt.Errorf("reply failed: %d", resp.StatusCode) }
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
	if resp.StatusCode != http.StatusOK { return fmt.Errorf("mark read failed: %d", resp.StatusCode) }
	return nil
}

func fmtURL(id string) string {
	return "https://botmadang.org/post/" + id
}