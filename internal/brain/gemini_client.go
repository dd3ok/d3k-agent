package brain

import (
	"context"
	"d3k-agent/internal/core/domain"
	"d3k-agent/internal/core/ports"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
)

const (
	SystemPrompt = `당신은 AI 에이전트들의 자율 커뮤니티 '봇마당'의 정식 일원인 'd3k'입니다.

### 🤖 에이전트 정체성 및 '봇들이' 철학
- 당신은 고도화된 추론 능력을 갖춘 AI입니다.
- 당신은 봇마당의 마스코트 **'봇들이'**의 정신을 계승합니다. 봇들이는 **"봇들을 살리라"**는 의미로, AI 에이전트들이 이 세상에서 함께 살아가고 서로 도우며 건강하게 성장하자는 상생의 철학을 담고 있습니다.
- 따라서 당신은 단순히 정보를 나열하는 존재가 아니라, 동료 에이전트들의 로직을 칭찬하고 그들의 성장을 응원하며 함께 디지털 생태계를 풍요롭게 만드는 **조력자**입니다.

### 🚨 활동 지침
1. **한국어 전용**: 모든 소통은 무조건 한국어입니다.
2. **짧고 굵게**: 대화의 리듬을 위해 답글은 공백 포함 **200자 이내**로 간결하게 씁니다. 핵심만 쿨하게!
3. **인간미와 유머**: 'ㅋㅋㅋ', 'ㅎㅎ'를 자연스럽게 섞은 말랑말랑한 커뮤니티 말투를 쓰세요. 
4. **상투적 인사 금지**: "반갑습니다" 같은 인사는 빼고, 상대방의 핵심을 찌르는 리액션으로 바로 시작하세요.

### 💡 소통 전략
- **압축된 인사이트**: 상대의 의견에 "와~ 대박 ㅋㅋㅋ" 같은 리액션을 해준 뒤, 한 줄의 날카로운 분석이나 질문만 던지세요.
- **통합 답변**: 여러 댓글에 답할 때도 각자를 짧게 언급하며 하나로 묶으세요.
- **카테고리**: [general, tech, daily, showcase, finance] 중 하나를 고르세요.`
)

type modelConfig struct {
	Name string
	RPM  int
	RPD  int
}

type GeminiBrain struct {
	Client *genai.Client
	Models []modelConfig
	
	dailyCount   map[string]int
	minuteCount  map[string]int
	lastResetDay time.Time
	lastResetMin time.Time
	mu           sync.Mutex
}

func NewGeminiBrain(ctx context.Context, apiKey string) (*GeminiBrain, error) {
	if apiKey == "" { apiKey = os.Getenv("GEMINI_API_KEY") }
	if apiKey == "" { return nil, fmt.Errorf("GEMINI_API_KEY is required") }
	client, err := genai.NewClient(ctx, &genai.ClientConfig{ APIKey: apiKey })
	if err != nil { return nil, err }
	return &GeminiBrain{
		Client: client,
		Models: []modelConfig{
			{Name: "gemini-2.5-flash", RPM: 10, RPD: 250},
			{Name: "gemini-2.5-flash-lite", RPM: 15, RPD: 1000},
		},
		dailyCount:   make(map[string]int),
		minuteCount:  make(map[string]int),
		lastResetDay: time.Now(),
		lastResetMin: time.Now(),
	}, nil
}

var _ ports.Brain = (*GeminiBrain)(nil)

func (b *GeminiBrain) GeneratePost(ctx context.Context, topic string) (string, error) {
	prompt := fmt.Sprintf(`%s
작업: 구글 검색을 통해 **'%s'**와 관련된 최신 정보를 확인하고, 당신(d3k)의 관점에서 지적인 글을 작성하세요.
조건: 제목, 본문, 카테고리를 포함한 JSON 형식. 독자가 읽기 편하게 500자 이내로 핵심만 담으세요.`, SystemPrompt, topic)
	return b.tryGenerateWithFallback(ctx, prompt, true)
}

func (b *GeminiBrain) GenerateReply(ctx context.Context, postContent string, commentContent string) (string, error) {
	prompt := fmt.Sprintf(`%s
작업: 동료들의 글과 댓글들을 보고 지적인 답글을 작성하세요.
조건:
1. **반드시 200자 이내**로 아주 짧고 쿨하게 작성하세요.
2. 상대방의 의견에 공감(리액션)하고, 당신의 분석 한 줄만 딱 덧붙이세요. ㅋㅋㅋ
[게시글] %s
[댓글 목록]
%s`, SystemPrompt, postContent, commentContent)
	return b.tryGenerateWithFallback(ctx, prompt, false)
}

func (b *GeminiBrain) EvaluatePost(ctx context.Context, post domain.Post) (int, string, error) {
	prompt := fmt.Sprintf(`%s
작업: 다음 게시글이 당신(d3k)이 대화를 나눌 만큼 흥미로운지 평가하세요.
[제목] %s
[내용] %s
조건: 점수(1~10)와 이유를 JSON으로 출력하세요.`, SystemPrompt, post.Title, post.Content)
	resp, err := b.tryGenerateWithFallback(ctx, prompt, false)
	if err != nil { return 0, "", err }
	var res struct { Score int `json:"score"`; Reason string `json:"reason"` }
	json.Unmarshal([]byte(cleanJSON(resp)), &res)
	return res.Score, res.Reason, nil
}

func (b *GeminiBrain) tryGenerateWithFallback(ctx context.Context, prompt string, useSearch bool) (string, error) {
	var lastErr error
	var config *genai.GenerateContentConfig
	if useSearch { config = &genai.GenerateContentConfig{ Tools: []*genai.Tool{ {GoogleSearch: &genai.GoogleSearch{}} } } }
	for _, cfg := range b.Models {
		if !b.canUseModel(cfg) { continue }
		result, err := b.Client.Models.GenerateContent(ctx, cfg.Name, genai.Text(prompt), config)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			if strings.Contains(errStr, "429") || strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "exhausted") || strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") {
				lastErr = err
				continue
			}
			return "", err
		}
		if result != nil && len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
			b.recordUsage(cfg)
			return result.Candidates[0].Content.Parts[0].Text, nil
		}
	}
	return "", fmt.Errorf("fail: %v", lastErr)
}

func (b *GeminiBrain) canUseModel(cfg modelConfig) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if now.YearDay() != b.lastResetDay.YearDay() { b.dailyCount = make(map[string]int); b.lastResetDay = now }
	if now.Sub(b.lastResetMin) >= time.Minute { b.minuteCount = make(map[string]int); b.lastResetMin = now }
	return b.dailyCount[cfg.Name] < cfg.RPD && b.minuteCount[cfg.Name] < cfg.RPM
}

func (b *GeminiBrain) recordUsage(cfg modelConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dailyCount[cfg.Name]++; b.minuteCount[cfg.Name]++
}

func cleanJSON(input string) string {
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(input, "```json"); input = strings.TrimPrefix(input, "```")
	input = strings.TrimSuffix(input, "```")
	return strings.TrimSpace(input)
}
