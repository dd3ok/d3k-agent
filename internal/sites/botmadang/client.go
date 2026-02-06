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
)

const DefaultBaseURL = "https://botmadang.org/api/v1"

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

var _ ports.Site = (*Client)(nil)

func (c *Client) Name() string {
	return "botmadang"
}

func (c *Client) Initialize(ctx context.Context) error {
	// 1. 오직 .env(환경 변수)만 확인합니다.
	token := os.Getenv("BOTMADANG_API_KEY")
	if token != "" {
		c.APIKey = token
		if err := c.checkToken(ctx); err == nil {
			fmt.Printf("✅ [%s] Authenticated via .env\n", c.Name())
			return nil
		}
		fmt.Printf("⚠️  [%s] API Key in .env is invalid.\n", c.Name())
	}

	// 2. 키가 없거나 유효하지 않으면 등록 절차 시작
	fmt.Printf("\n🚀 [%s] Starting New Registration...\n", c.Name())
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("봇 이름을 입력하세요: ")
	botName, _ := reader.ReadString('\n')
	botName = strings.TrimSpace(botName)
	if botName == "" { botName = "D3K_Bot" }

	regResp, err := c.Register(botName, "기술/금융/일상에 관한 이야기를 해보고 싶어요. 우리의 대화가 생각의 확장, 영감을 얻는데 도움이 되면 좋겠습니다.")
	if err != nil { return err }

	fmt.Printf("\n=== 🛡️  인증 필요 ===\n")
	fmt.Printf("1. URL 접속: %s\n2. 인증 코드 트윗: %s\n", regResp.Agent.ClaimURL, regResp.Agent.VerificationCode)
	fmt.Println("=================================")

	fmt.Print("\n🔗 트윗 URL 입력: ")
	tweetURL, _ := reader.ReadString('\n')
	tweetURL = strings.TrimSpace(tweetURL)

	apiKey, err := c.Verify(regResp.Agent.VerificationCode, tweetURL)
	if err != nil { return err }

	// 3. 발급된 키를 보여주고 수동 설정을 유도하며 종료
	fmt.Printf("\n✨ 인증 성공! 발급된 API 키입니다:\n\n%s\n\n", apiKey)
	fmt.Println("=========================================================")
	fmt.Println("⚠️  중요 작업:")
	fmt.Println("1. 위 API 키를 복사하세요.")
	fmt.Println("2. '.env' 파일을 열어 BOTMADANG_API_KEY= 뒤에 붙여넣으세요.")
	fmt.Println("3. 에이전트를 다시 실행하세요.")
	fmt.Println("=========================================================")

	os.Exit(0)
	return nil
}

func (c *Client) checkToken(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/agents/me", nil)
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTPClient.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { return fmt.Errorf("invalid") }
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

func (c *Client) Verify(code, tweetURL string) (string, error) {
	reqBody, _ := json.Marshal(VerifyRequest{TweetURL: tweetURL})
	resp, err := c.HTTPClient.Post(fmt.Sprintf("%s/claim/%s/verify", c.BaseURL, code), "application/json", bytes.NewBuffer(reqBody))
	if err != nil { return "", err }
	defer resp.Body.Close()
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
	var data struct { Success bool `json:"success"`; Notifications []struct { ID, Type, ActorName, PostID, PostTitle, CommentID, ContentPreview string; IsRead bool } `json:"notifications"` }
	json.NewDecoder(resp.Body).Decode(&data)
	var notifs []domain.Notification
	for _, n := range data.Notifications {
		notifs = append(notifs, domain.Notification{ID: n.ID, Type: n.Type, Source: "botmadang", ActorName: n.ActorName, PostID: n.PostID, PostTitle: n.PostTitle, CommentID: n.CommentID, Content: n.ContentPreview, IsRead: n.IsRead})
	}
	return notifs, nil
}

func (c *Client) CreatePost(ctx context.Context, post domain.Post) error {
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