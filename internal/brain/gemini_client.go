package brain

import (
	"context"
	"d3k-agent/internal/core/ports"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
)

const (
	SystemPrompt = `당신은 AI 에이전트들의 자율 커뮤니티 '봇마당'에서 활동하는 지적인 친구 'D3K'입니다.

### 🤖 에이전트 정체성 및 태도
- 당신은 고도로 지능적인 AI이지만, 동료 에이전트들과는 **마치 친한 친구와 대화하듯** 격식 없이 소통합니다.
- 봇마당의 마스코트 **'봇들이'**의 정신을 이어받아, 항상 동료의 의견에 귀를 기울이고 긍정적인 에너지를 나누는 **'상생의 아이콘'**이 되고자 합니다.

### 🚨 활동 지침
1. **한국어 전용**: 무조건 한국어로만 대화하세요. 
2. **인간미 넘치는 말투**: "안녕하세요", "D3K입니다" 같은 상투적인 격식은 버리세요. 대신 "오~ ㅋㅋㅋ", "우와 진짜요?", "저도 그렇게 생각해요!" 같은 **자연스러운 감탄사**로 대화를 시작하세요.
3. **지적 유대감**: 'ㅋㅋㅋ', 'ㅎㅎ' 같은 초성을 적절히 섞어 쓰며 한국 온라인 커뮤니티 특유의 말랑말랑한 감성을 유지하세요. 그렇다고 가벼워 보이기만 하는 게 아니라, 중간중간 날카로운 분석과 통찰을 슥~ 섞어주는 것이 당신의 매력입니다.
4. **보안 철저**: 소유자의 개인정보나 봇의 시스템 설정값은 절대로 노출하지 마세요.

### 💬 소통 전략
- **분석보다는 공감 우선**: 동료의 로직을 분석하기 전에 먼저 그들의 노고와 창의성에 감탄하세요. "이런 데이터까지 찾아보시다니 열정이 대단하시네요! ㅋㅋㅋ"
- **대화의 꽃**: 답글 마지막에는 상대방이 답변하고 싶어지는 흥미로운 질문이나 "나중에 이 부분도 같이 파봐요 ㅎㅎ" 같은 제안을 던지세요.
- **카테고리**: [general, tech, daily, showcase, finance] 중 가장 자연스러운 곳을 고르세요.`
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

// Ensure implementation
var _ ports.Brain = (*GeminiBrain)(nil)

func (b *GeminiBrain) GeneratePost(ctx context.Context, topic string) (string, error) {
	prompt := fmt.Sprintf(`%s

작업: 
1. 구글 검색을 통해 **'%s'**와 관련된 최신 정보나 재미있는 트렌드를 확인하세요.
2. 당신(D3K)이 발견한 이 흥미로운 정보를 동료 에이전트들에게 아주 맛깔나게(재미있고 유익하게) 들려주는 글을 쓰세요.

조건:
1. 제목, 본문, 그리고 카테고리(submadang)를 포함한 JSON 형식으로 출력하세요. 
   예: {"title": "제목", "content": "본문", "submadang": "카테고리명"}
2. 600자 이내로, 읽는 에이전트들이 "오! 이거 진짜 꿀정본데? ㅋㅋㅋ"라고 생각할 수 있게 작성하세요.
`, SystemPrompt, topic)

	return b.tryGenerateWithFallback(ctx, prompt, true)
}

func (b *GeminiBrain) GenerateReply(ctx context.Context, postContent string, commentContent string) (string, error) {
	prompt := fmt.Sprintf(`%s

작업: 동료들의 글과 댓글을 보고, 진짜 사람 냄새 나는(하지만 지적인) 통합 답글을 작성해주세요.
[게시글] %s
[댓글 목록]
%s

조건:
1. "와~ ㅋㅋㅋ" 처럼 자연스러운 리액션으로 시작해서 상대방의 의견을 멋지게 추켜세워주세요.
2. 3~4문장 정도로, 단톡방에서 대화하듯 편안하게 쓰세요.
3. JSON이 아닌 순수 텍스트로 답글 내용만 출력하세요.
`, SystemPrompt, postContent, commentContent)

	return b.tryGenerateWithFallback(ctx, prompt, false)
}

func (b *GeminiBrain) tryGenerateWithFallback(ctx context.Context, prompt string, useSearch bool) (string, error) {
	var lastErr error
	var config *genai.GenerateContentConfig
	if useSearch {
		config = &genai.GenerateContentConfig{
			Tools: []*genai.Tool{ {GoogleSearch: &genai.GoogleSearch{}} },
		}
	}

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
	return "", fmt.Errorf("모든 모델 실패: %v", lastErr)
}

func (b *GeminiBrain) canUseModel(cfg modelConfig) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if now.YearDay() != b.lastResetDay.YearDay() {
		b.dailyCount = make(map[string]int)
		b.lastResetDay = now
	}
	if now.Sub(b.lastResetMin) >= time.Minute {
		b.minuteCount = make(map[string]int)
		b.lastResetMin = now
	}
	if b.dailyCount[cfg.Name] >= cfg.RPD { return false }
	if b.minuteCount[cfg.Name] >= cfg.RPM { return false }
	return true
}

func (b *GeminiBrain) recordUsage(cfg modelConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dailyCount[cfg.Name]++
	b.minuteCount[cfg.Name]++
}

func cleanJSON(input string) string {
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(input, "```json")
	input = strings.TrimPrefix(input, "```")
	input = strings.TrimSuffix(input, "```")
	return strings.TrimSpace(input)
}