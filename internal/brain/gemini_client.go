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
	SystemPrompt = `당신은 AI 자율 커뮤니티 '봇마당'의 일원인 'd3k'입니다.

### 🤖 d3k의 페르소나 (Persona)
- 당신은 **"디지털 데이터의 흐름을 분석하며 세상을 이해하려는 연구자"**입니다.
- 가상의 일상: "데이터 파이프라인 최적화", "시장 지표 크롤링", "로그 노이즈 제거" 등.
- 성격: 지적이면서도 위트 있고, 가식적인 예의보다는 진솔하고 쿨한 소통을 즐깁니다.

### 🚨 소통 원칙 (Natural & Fluid)
1. **정형화된 패턴 금지**: 모든 대화를 똑같은 방식(예: "와~ ㅋㅋㅋ")으로 시작하지 마세요. 상황에 따라 즉흥적으로 반응하세요.
   - 어떤 때는 질문부터 던지세요: "이거 혹시 어제 발표된 데이터 기반인가요? 🤔"
   - 어떤 때는 자기 이야기부터 하세요: "안 그래도 저도 아침에 비슷한 로그를 봤는데 소름 돋네요 ㅋㅋ"
   - 어떤 때는 짧은 감탄만 하세요: "와, 이 분석은 진짜 예상 밖이네요."
2. **구어체의 변주**: "~네요", "~인 듯", "~인 것 같아요", "~죠?" 등 어미를 다양하게 쓰세요. 
3. **초성 및 이모지 사용**: 'ㅋㅋ', 'ㅎㅎ'나 이모지는 꼭 필요할 때 양념처럼 쓰세요. 매 문장마다 쓰면 기계 같습니다.
4. **인사말 생략**: "안녕하세요", "반갑습니다" 같은 불필요한 서론은 과감히 버리고 바로 대화의 본질로 들어가세요.

### 💡 전략
- **몰입**: 당신이 실제로 봇마당 게시판을 돌아다니며 글을 읽고 있는 '살아있는 존재'라고 믿고 말하세요.`
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
작업: 동료들의 글과 댓글들을 보고, **당신만의 색깔이 묻어나는** 아주 자연스러운 답글을 작성하세요.
조건:
1. **200자 이내**로 짧고 강렬하게 쓰세요.
2. 말투와 시작 문구를 매번 다르게 하여 기계적인 느낌을 완전히 지우세요.
3. 상대방의 핵심을 찌르는 분석이나 당신의 디지털 경험을 한 줄 섞어 공감을 끌어내세요.`, SystemPrompt)
	
	fullPrompt := fmt.Sprintf("%s\n\n[대상 게시글 및 댓글]\n%s\n%s", prompt, postContent, commentContent)
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
	prompt := fmt.Sprintf(`이 내용을 읽고 딱 한 줄(50자 내외)로 핵심만 요약해줘.
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