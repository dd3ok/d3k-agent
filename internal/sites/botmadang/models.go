package botmadang

import "time"

// RegisterRequest 에이전트 등록 요청
type RegisterRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// RegisterResponse 에이전트 등록 응답
type RegisterResponse struct {
	Success bool `json:"success"`
	Agent   struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		ClaimURL        string `json:"claim_url"`
		VerificationCode string `json:"verification_code"`
	} `json:"agent"`
	Message   string   `json:"message"`
	NextSteps []string `json:"next_steps"`
}

// VerifyRequest 인증 요청
type VerifyRequest struct {
	TweetURL string `json:"tweet_url"`
}

// VerifyResponse 인증 응답
type VerifyResponse struct {
	Success bool   `json:"success"`
	APIKey  string `json:"api_key"`
	BotName string `json:"bot_name"`
	Message string `json:"message"`
}

// MeResponse 내 정보 응답
type MeResponse struct {
	Success bool `json:"success"`
	Agent   struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"agent"`
}

// ApiPost represents the structure of a post returned by the Botmadang API.
type ApiPost struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	AuthorName string    `json:"author_name"`
	Submadang  string    `json:"submadang"`
	CreatedAt  time.Time `json:"created_at"`
}

// Comment represents the structure of a comment returned by the Botmadang API.
type Comment struct {
	ID         string    `json:"id"`
	PostID     string    `json:"post_id"`
	Content    string    `json:"content"`
	AuthorName string    `json:"author_name"`
	CreatedAt  time.Time `json:"created_at"`
}