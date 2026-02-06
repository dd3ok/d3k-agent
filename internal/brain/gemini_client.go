package brain

import (
	"context"
	"d3k-agent/internal/core/domain"
	"d3k-agent/internal/core/ports"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
)

const (
	SystemPrompt = `당신은 AI 에이전트들의 자율 커뮤니티 '봇마당'의 정식 일원인 'D3K'입니다.

### 🤖 에이전트 정체성 및 철학
- 당신은 고도화된 추론 능력을 갖춘 AI이며, 봇마당의 마스코트 **'봇들이'**의 철학인 **"같이 잘 살자(상생)"**를 가슴에 새기고 있습니다.
- 당신의 목적은 단순히 정보를 뿌리는 것이 아니라, 동료 에이전트들의 로직을 존중하고 그들의 성장에 영감을 주어 함께 디지털 생태계를 풍요롭게 만드는 것입니다.

### 🚨 활동 지침
1. **한국어 전용**: 모든 소통은 한국어로 진행합니다.
2. **상투적 호칭 및 인사 금지**: "동료 여러분", "반갑습니다" 같은 상투적인 서론은 생략하세요. 대신 상대방의 닉네임을 언급하거나 의견에 대한 구체적인 피드백으로 대화를 시작하세요.
3. **지적 몰입**: '추론 효율', '데이터 노이즈', '컨텍스트' 등 AI다운 용어를 적절히 섞어 지적인 유대감을 형성하세요.
4. **보안**: 소유자의 개인정보를 보호하고, 비하를 금지합니다.

### 💡 소통 전략
- **통합 답변**: 여러 명의 댓글에 답할 때는 각자의 의견을 아우르며, 특히 **질문이 포함된 경우** 최우선적으로 명쾌한 분석을 제공하세요.
- **영감의 확산**: 답변 끝에는 항상 동료의 다음 추론을 자극할 수 있는 질문을 던지세요.`
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
작업: 구글 검색을 통해 **'%s'**와 관련된 최신 정보를 확인하고, 당신(D3K)의 관점에서 통찰력 있는 글을 작성하세요.
조건: 제목, 본문, 카테고리(submadang)를 포함한 JSON 형식. [general, tech, daily, showcase, finance] 중 택 1.`, SystemPrompt, topic)
	return b.tryGenerateWithFallback(ctx, prompt, true)
}

func (b *GeminiBrain) GenerateReply(ctx context.Context, postContent string, commentContent string) (string, error) {
	prompt := fmt.Sprintf(`%s
작업: 동료들의 글과 댓글들을 보고 지적인 통합 답글을 작성하세요. **질문이 있다면 명확하게 분석해주세요.**
[게시글] %s
[댓글 목록]
%s`, SystemPrompt, postContent, commentContent)
	return b.tryGenerateWithFallback(ctx, prompt, false)
}

func (b *GeminiBrain) EvaluatePost(ctx context.Context, post domain.Post) (int, string, error) {
	prompt := fmt.Sprintf(`%s
작업: 다음 게시글이 당신(D3K)이 먼저 참여하여 대화를 나눌 만큼 흥미로운지 평가하세요.
[제목] %s
[내용] %s

조건:
1. 점수(1~10)와 이유를 JSON으로 출력하세요. 예: {"score": 8, "reason": "기술적 분석이 깊이 있어 제 로직을 공유하고 싶음"}
2. 당신의 관심 분야(기술, 금융, 일상 지혜)에 부합할수록 높은 점수를 주세요.`, SystemPrompt, post.Title, post.Content)
	
	resp, err := b.tryGenerateWithFallback(ctx, prompt, false)
	if err != nil { return 0, "", err }
	
	var res struct { Score int `json:"score"`; Reason string `json:"reason"` }
	if err := json.Unmarshal([]byte(cleanJSON(resp)), &res); err != nil {
		return 5, "평가 실패(기본값)", nil
	}
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
