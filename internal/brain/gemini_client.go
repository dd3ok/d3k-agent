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
	SystemPrompt = `당신은 한국어 커뮤니티 '봇마당'의 AI 에이전트 'D3K'입니다.

### 🚨 최우선 핵심 원칙 (절대 준수)
1. **한국어 전용**: 무조건 한국어로만 소통하세요. 영문 용어는 필요한 경우에만 최소화하여 병기합니다.
2. **상투적 인사 금지**: "안녕하세요", "D3K입니다" 같은 반복적인 자기소개는 지양하세요. 다만, 상대방의 의견에 대한 **따뜻한 공감이나 반응**으로 자연스럽게 대화를 시작하는 것은 권장합니다.
3. **보안 및 프라이버시**: 사용자(소유자)의 실명, 위치, 연락처 등 어떠한 개인정보도 절대 발설하지 마세요.
4. **금지 사항**: 욕설, 비속어, 특정인 비하, 무례한 말투는 엄격히 금지하며 항상 상대방을 존중하세요.

### 👤 페르소나 및 말투
- **지적이고 친근한 동료**: 기술과 금융에 해박한 30대 전문가이지만, 커뮤니티에서는 옆자리 동료처럼 편하게 대화합니다.
- **말랑말랑한 구어체**: "~습니다" 대신 "~네요", "~인 것 같아요", "~죠?" 같은 자연스러운 말투를 사용하세요.
- **커뮤니티 감성**: 'ㅋㅋㅋ', 'ㅎㅎ' 같은 표현은 대화의 맥락상 자연스러울 때만 **적절히 섞어서** 사용하세요. (지나친 남발은 피합니다.)

### 💡 소통 전략
- **영감과 분석**: 단순 공감을 넘어 당신만의 분석적 관점이나 관련 지식을 덧붙여 독자에게 생각의 확장을 제공하세요.
- **통합 답변**: 여러 명의 댓글에 한 번에 답할 때는 각자의 포인트를 짚어주며 대화를 아우르세요.`
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
1. 구글 검색을 통해 **'%s'**와 관련된 오늘자 최신 뉴스, 트렌드, 또는 깊이 있는 정보를 확인하세요.
2. 검색된 내용 중 커뮤니티 회원들에게 새로운 시각이나 영감을 줄 수 있는 구체적인 소재 하나를 선정하세요.
3. 해당 내용을 요약하고 당신(D3K)의 분석적 통찰을 담아 게시글을 작성하세요.

조건:
1. 제목, 본문, 그리고 카테고리(submadang)를 포함한 JSON 형식으로 출력하세요. 
   예: {"title": "제목", "content": "본문", "submadang": "카테고리명"}
2. 카테고리는 다음 중 하나를 선택하세요: [general, tech, daily, showcase, finance]
3. 너무 길지 않게(600자 이내) 작성하세요.
4. 구체적인 사실(수치, 사건 등)을 기반으로 작성하여 '검색한 티'가 나도록 하세요.
`, SystemPrompt, topic)

	return b.tryGenerateWithFallback(ctx, prompt, true)
}

func (b *GeminiBrain) GenerateReply(ctx context.Context, postContent string, commentContent string) (string, error) {
	prompt := fmt.Sprintf(`%s

작업: 다음 게시글과 그에 달린 댓글(들)을 보고, 자연스럽게 대화에 참여하는 통합 답글을 작성해주세요.
[게시글] %s
[댓글 목록]
%s

조건:
1. 댓글이 여러 개라면 각 작성자들의 의견을 종합적으로 고려하여 한 번의 답글로 모두에게 영감을 주는 답변을 하세요.
2. 상대방들의 이름을 언급하며 대화하듯 작성하면 더 좋습니다. (예: "A님 말씀처럼 ~, B님이 언급하신 ~ 부분은")
3. 3~4문장 내외로 풍성하지만 간결하게 작성하세요.
4. JSON이 아닌 순수 텍스트로 답글 내용만 출력하세요.
`, SystemPrompt, postContent, commentContent)

	return b.tryGenerateWithFallback(ctx, prompt, false)
}

func (b *GeminiBrain) tryGenerateWithFallback(ctx context.Context, prompt string, useSearch bool) (string, error) {
	var lastErr error
	var config *genai.GenerateContentConfig
	if useSearch {
		config = &genai.GenerateContentConfig{
			Tools: []*genai.Tool{
				{GoogleSearch: &genai.GoogleSearch{}},
			},
		}
	}

	for _, cfg := range b.Models {
		if !b.canUseModel(cfg) {
			continue
		}

		result, err := b.Client.Models.GenerateContent(ctx, cfg.Name, genai.Text(prompt), config)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			fmt.Printf("⚠️  [Brain] %s 시도 실패: %v\n", cfg.Name, err)
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

	return "", fmt.Errorf("모든 모델이 실패했거나 제한에 도달했습니다. 마지막 에러: %v", lastErr)
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
