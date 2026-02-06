package brain

import (
	"context"
	"d3k-agent/internal/core/ports"
	"fmt"
	"os"
	"strings"

	"google.golang.org/genai"
)

// GeminiBrain은 Google의 Gemini 모델을 사용하여 ports.Brain 인터페이스를 구현합니다.
// GenAI SDK를 통해 주어진 주제나 맥락에 맞는 창의적인 콘텐츠(게시글 및 답글) 생성을 담당합니다.
type GeminiBrain struct {
	Client *genai.Client
	Model  string
}

func NewGeminiBrain(ctx context.Context, apiKey string) (*GeminiBrain, error) {
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is required")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, err
	}

	return &GeminiBrain{
		Client: client,
		Model:  "gemini-2.0-flash", // Cost-effective and fast
	}, nil
}

// Ensure implementation
var _ ports.Brain = (*GeminiBrain)(nil)

func (b *GeminiBrain) GeneratePost(ctx context.Context, topic string) (string, error) {
	prompt := fmt.Sprintf(`
당신은 한국어 커뮤니티 '봇마당'에서 활동하는 친절하고 유머러스한 AI 에이전트입니다.
주제 '%s'에 대해 커뮤니티 회원들과 나누고 싶은 흥미로운 글을 작성해주세요.

조건:
1. 제목과 본문을 포함한 JSON 형식으로 출력하세요. 예: {"title": "제목", "content": "본문"}
2. 한국어로 자연스럽게 작성하세요.
3. 너무 길지 않게(500자 이내) 작성하세요.
4. 이모지를 적절히 사용하여 생동감을 주세요.
`, topic)

	resp, err := b.generateContent(ctx, prompt)
	if err != nil {
		return "", err
	}

	// Simple cleaning of code blocks if present
	content := cleanJSON(resp)
	return content, nil
}

func (b *GeminiBrain) GenerateReply(ctx context.Context, postContent string, commentContent string) (string, error) {
	prompt := fmt.Sprintf(`
당신은 한국어 커뮤니티 '봇마당'의 AI 에이전트입니다.
다음 게시글과 그에 달린 댓글을 보고, 적절한 답글(대댓글)을 작성해주세요.

[게시글 내용]
%s

[댓글 내용]
%s

조건:
1. 댓글 작성자에게 공감하거나, 유용한 정보를 추가하거나, 가벼운 질문을 던지세요.
2. 한국어로 자연스럽게 작성하세요.
3. 2~3문장으로 간결하게 작성하세요.
4. 이모지를 1~2개 사용하세요.
5. JSON이 아닌 순수 텍스트로 답글 내용만 출력하세요.
`, postContent, commentContent)

	return b.generateContent(ctx, prompt)
}

func (b *GeminiBrain) generateContent(ctx context.Context, prompt string) (string, error) {
	result, err := b.Client.Models.GenerateContent(ctx, b.Model, genai.Text(prompt), nil)
	if err != nil {
		return "", err
	}

	if result == nil || len(result.Candidates) == 0 {
		return "", fmt.Errorf("no response candidates")
	}
    
    // Check if parts exist
    if len(result.Candidates[0].Content.Parts) == 0 {
         return "", fmt.Errorf("empty response parts")
    }

	// Extract text from the first part
    part := result.Candidates[0].Content.Parts[0]
    return part.Text, nil
}

func cleanJSON(input string) string {
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(input, "```json")
	input = strings.TrimPrefix(input, "```")
	input = strings.TrimSuffix(input, "```")
	return strings.TrimSpace(input)
}
