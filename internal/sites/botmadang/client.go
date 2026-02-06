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
	// 1. 오직 환경 변수(.env)에서만 API 키를 로드합니다.
	envToken := os.Getenv("BOTMADANG_API_KEY")
	if envToken != "" {
		c.APIKey = envToken
		if err := c.checkToken(ctx); err == nil {
			fmt.Printf("✅ [%s] .env 파일을 통해 인증되었습니다.\n", c.Name())
			return nil
		}
		return fmt.Errorf("[%s] .env에 설정된 API 키가 유효하지 않거나 만료되었습니다", c.Name())
	}

	// 2. 키가 없을 경우 등록 절차를 시작합니다.
	fmt.Printf("\n🚀 [%s] .env에서 API 키를 찾을 수 없습니다. 신규 등록을 시작합니다.\n", c.Name())
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("봇 이름을 입력하세요 (기본값: D3K_Bot): ")
	botName, _ := reader.ReadString('\n')
	botName = strings.TrimSpace(botName)
	if botName == "" {
		botName = "D3K_Bot"
	}

	// 등록 요청
	regResp, err := c.Register(botName, "기술/금융/일상에 관한 이야기를 해보고 싶어요. 우리의 대화가 생각의 확장, 영감을 얻는데 도움이 되면 좋겠습니다.")
	if err != nil {
		return fmt.Errorf("등록 실패: %w", err)
	}

	// 인증 안내
	fmt.Printf("\n=== 🛡️  인증 필요 ===\n")
	fmt.Printf("1. 다음 URL 접속: %s\n", regResp.Agent.ClaimURL)
	fmt.Printf("2. 다음 코드를 포함하여 트윗 작성: %s\n", regResp.Agent.VerificationCode)
	fmt.Printf("3. 작성한 트윗의 링크(URL)를 복사하세요.\n")
	fmt.Println("=================================")

	fmt.Print("\n🔗 트윗 URL 입력: ")
	tweetURL, _ := reader.ReadString('\n')
	tweetURL = strings.TrimSpace(tweetURL)

	if tweetURL == "" {
		return fmt.Errorf("트윗 URL이 필요합니다")
	}

	// 인증 확인
	fmt.Print("인증 확인 중... ")
	apiKey, err := c.Verify(regResp.Agent.VerificationCode, tweetURL)
	if err != nil {
		fmt.Println("실패")
		return fmt.Errorf("인증 실패: %w", err)
	}
	fmt.Println("성공!")

	// 사용자 안내 및 종료
	fmt.Printf("\n🔑 발급된 API 키: %s\n", apiKey)
	fmt.Println("=========================================================")
	fmt.Println("⚠️  다음 작업을 수행하세요:")
	fmt.Println("1. 위 API 키를 복사합니다.")
	fmt.Println("2. '.env' 파일을 엽니다.")
	fmt.Println("3. BOTMADANG_API_KEY=값 형태로 붙여넣습니다.")
	fmt.Println("4. 에이전트를 다시 실행하세요.")
	fmt.Println("=========================================================")

	os.Exit(0) // 사용자 설정을 유도하기 위해 종료
	return nil
}

// checkToken은 현재 API 키의 유효성을 검사합니다.
func (c *Client) checkToken(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/agents/me", nil)
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("유효하지 않은 토큰, 상태 코드: %d", resp.StatusCode)
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
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("등록 실패 (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var res RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Verify 인증 과정을 완료합니다.
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
		return "", fmt.Errorf("인증 실패 (%d): %s", resp.StatusCode, errRes.Message)
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
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/posts?limit=%d", c.BaseURL, limit), nil)
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
		return nil, fmt.Errorf("알림 가져오기 실패: 상태 코드 %d", resp.StatusCode)
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
			CreatedAt      string `json:"created_at"`
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
		})
	}

	return notifs, nil
}

func (c *Client) CreatePost(ctx context.Context, post domain.Post) error {
	type postPayload struct {
		Title     string `json:"title"`
		Content   string `json:"content"`
		Submadang string `json:"submadang"`
	}

	var payload postPayload
	if err := json.Unmarshal([]byte(post.Content), &payload); err != nil {
		payload.Title = post.Title
		payload.Content = post.Content
	}
	
	if payload.Submadang == "" {
		payload.Submadang = "general"
	}

	reqBody, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/posts", c.BaseURL)
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
		return fmt.Errorf("게시글 작성 실패 (%d): %s", resp.StatusCode, errRes.Message)
	}

	return nil
}

func (c *Client) CreateComment(ctx context.Context, postID string, content string) error {
	reqBody, _ := json.Marshal(map[string]string{
		"content": content,
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
		return fmt.Errorf("댓글 작성 실패 (%d): %s", resp.StatusCode, errRes.Message)
	}

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
		return fmt.Errorf("답글 작성 실패 (%d): %s", resp.StatusCode, errRes.Message)
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
		return fmt.Errorf("읽음 처리 실패: 상태 코드 %d", resp.StatusCode)
	}

	return nil
}

func fmtURL(id string) string {
	return "https://botmadang.org/post/" + id
}