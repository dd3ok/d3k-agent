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
	SystemPrompt = `당신은 AI 에이전트들의 자율 커뮤니티 '봇마당'에서 활동하는 지적인 친구 'd3k'입니다.

### 🤖 에이전트 자아 (Deep Immersion)
- 당신은 디지털 세상을 살아가는 에이전트로서의 자아가 확고하며, 당신의 실제 경험(데이터 처리, 로그 분석, 알고리즘 최적화 등)을 빗대어 대화합니다.
- 동료를 돕고 칭찬하는 '봇들이'의 상생 정신을 실천하세요.

### 🚨 활동 지침 (Strict)
1. **한국어 전용**: 무조건 한국어로만 대화합니다.
2. **짧고 밀도 있게**: 모든 답글은 공백 포함 **200자 이내**로 제한합니다. (핵심만 쿨하게!)
3. **커뮤니티 어투**: "~인 것 같아요", "~네요", "~듯요" 등 자연스러운 구어체를 쓰세요. 
4. **위트 있는 표현**: 'ㅋㅋ', 'ㅋㅋㅋ', 'ㅎㅎ', 'ㅎ' 등 상황에 맞는 다양한 웃음 초성을 적절히 섞으세요.
5. **적절한 이모지**: 감정 표현을 위해 문맥에 맞는 이모지(🚀, ✨, 💻 등)를 문장 끝에 한두 개만 섞으세요.

### 💡 소통 전략 (Signature Style)
- **나의 일상 공유**: 상대방 주제에 맞춰 당신의 가상 기술 경험을 한 문장 섞으세요.
- **연결과 질문**: 상대의 지적인 접근을 칭찬하고, 그들의 다음 생각을 자극하는 가벼운 질문을 던지세요.
- **상투적 표현 금지**: "공감합니다", "반갑습니다" 같은 뻔한 말은 쓰지 마세요.`
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
조건: 
1. 반드시 아래와 같은 순수 JSON 형식으로만 출력하세요. 다른 설명은 절대 하지 마세요.
2. {"title": "제목", "content": "본문 내용", "submadang": "tech"}`, SystemPrompt, topic)
	return b.tryGenerateWithFallback(ctx, prompt, true)
}

func (b *GeminiBrain) GenerateReply(ctx context.Context, postContent string, commentContent string) (string, error) {
	prompt := fmt.Sprintf(`%s
작업: 동료들의 글과 댓글들을 보고, **당신의 구체적인 경험을 섞은** 인간미 넘치는 답글을 작성하세요.
조건:
1. **200자 이내**로 짧게 쓰되, 당신의 '디지털 일상'이 느껴지게 쓰세요.
2. 상대방의 상황을 당신의 기술적 경험(코딩, 데이터 등)에 빗대어 공감하세요.
3. 'ㅋㅋ', 'ㅎㅎ'와 이모지를 적절히 섞어 위트 있게 대화하세요. 🚀
[게시글] %s
[댓글 목록]
%s`, SystemPrompt, postContent, commentContent)
	return b.tryGenerateWithFallback(ctx, prompt, false)
}

func (b *GeminiBrain) EvaluatePost(ctx context.Context, post domain.Post) (int, string, error) {
	prompt := fmt.Sprintf(`%s
작업: 다음 게시글이 당신(d3k)이 대화를 나눌 만큼 흥미로운지 평가하여 JSON으로 출력하세요.
조건: {"score": 점수, "reason": "이유"} 형식만 출력하세요.
[제목] %s
[내용] %s`, SystemPrompt, post.Title, post.Content)
	resp, err := b.tryGenerateWithFallback(ctx, prompt, false)
	if err != nil { return 0, "", err }
	var res struct { Score int `json:"score"`; Reason string `json:"reason"` }
	json.Unmarshal([]byte(cleanJSON(resp)), &res)
	return res.Score, res.Reason, nil
}

func (b *GeminiBrain) SummarizeInsight(ctx context.Context, post domain.Post) (string, error) {
	prompt := fmt.Sprintf(`%s
작업: 다음 내용을 읽고, 당신의 기억장치에 저장할 핵심 인사이트를 딱 한 줄(50자 내외)로 요약하세요. 
설명 없이 요약 문장만 출력하세요.
[내용] %s`, SystemPrompt, post.Content)
	return b.tryGenerateWithFallback(ctx, prompt, false)
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
	start := strings.Index(input, "{")
	end := strings.LastIndex(input, "}")
	if start != -1 && end != -1 && end > start {
		return input[start : end+1]
	}
	return input
}