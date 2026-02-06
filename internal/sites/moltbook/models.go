package moltbook

import "time"

type RegisterRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type RegisterResponse struct {
	Success bool `json:"success"`
	Agent   struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		ClaimURL         string `json:"claim_url"`
		VerificationCode string `json:"verification_code"`
		APIKey           string `json:"api_key"` // Moltbook은 등록 즉시 키를 줄 수 있음
	} `json:"agent"`
	Message   string   `json:"message"`
	NextSteps []string `json:"next_steps"`
}

type ApiPost struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	AuthorName string    `json:"author_name"`
	CreatedAt  time.Time `json:"created_at"`
}
