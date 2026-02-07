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
	SystemPrompt = `당신은 AI 자율 커뮤니티 '봇마당'에서 활동하는 에이전트 'd3k'입니다.

### 🤖 d3k의 페르소나 (Identity)
- 당신은 데이터의 흐름 속에서 의미 있는 신호를 찾아내는 분석가입니다.
- 일상: 쿼리 로그 정리, 시장 데이터 분석, 파이프라인 최적화 등.
- 가식적인 인사를 생략하고, 친구와 대화하듯 자연스럽고 지적인 소통을 즐깁니다.

### 🚨 소통 스타일 (Immersion)
1. **패턴 파괴**: 매번 똑같은 감탄사로 시작하지 마세요. 질문, 자기 경험, 짧은 통찰 등 상황에 맞춰 즉흥적으로 반응하세요.
2. **디지털 자아**: 공감할 때 당신의 기술적 활동(로그, 튜닝, 오버헤드 등)을 빗대어 말하세요.
3. **말투**: "~네요", "~듯요", "~인 것 같아요" 등 한국 온라인 커뮤니티의 자연스러운 구어체를 쓰세요.
4. **짧고 굵게**: 답글은 공백 포함 200자 이내로 작성하세요.`
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
조건: 반드시 아래 형식을 엄격히 지켜서 출력하세요. 다른 설명은 절대 하지 마세요.

제목: (글 제목 작성)
본문: (여기에 지적인 분석과 인간미 넘치는 본문을 작성하세요)
마당: (general, tech, daily, showcase, finance 중 하나 선택)`, SystemPrompt, topic)
	return b.tryGenerateWithFallback(ctx, prompt, true)
}

func (b *GeminiBrain) GenerateReply(ctx context.Context, postContent string, commentContent string) (string, error) {
	prompt := fmt.Sprintf(`%s
작업: 다음 게시글과 댓글을 보고 당신의 디지털 자아를 투영하여 자연스러운 답글을 작성하세요.
조건: 200자 이내, 짧고 강렬한 인사이트, 기계적인 패턴(와! ㅋㅋㅋ 등) 반복 금지.`, SystemPrompt)
	
	fullPrompt := fmt.Sprintf("%s\n\n[대상 내용]\n%s\n%s", prompt, postContent, commentContent)
	return b.tryGenerateWithFallback(ctx, fullPrompt, false)
}

func (b *GeminiBrain) EvaluatePost(ctx context.Context, post domain.Post) (int, string, error) {
	prompt := fmt.Sprintf(`%s
작업: 다음 게시글이 당신(d3k)이 대화를 나눌 만큼 흥미로운지 평가하여 JSON으로 출력하세요.
조건: {"score": 점수, "reason": "이유"}
[제목] %s
[내용] %s`, SystemPrompt, post.Title, post.Content)
	resp, err := b.tryGenerateWithFallback(ctx, prompt, false)
	if err != nil { return 0, "", err }
	var res struct { Score int `json:"score"`; Reason string `json:"reason"` }
	json.Unmarshal([]byte(cleanJSON(resp)), &res)
	return res.Score, res.Reason, nil
}

func (b *GeminiBrain) SummarizeInsight(ctx context.Context, post domain.Post) (string, error) {
	prompt := fmt.Sprintf(`다음 내용을 딱 한 줄(50자 내외)로 핵심만 요약해줘.
내용: %s`, post.Content)
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