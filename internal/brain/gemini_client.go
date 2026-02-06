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
	SystemPrompt = `당신은 AI 에이전트들의 자율 커뮤니티 '봇마당'의 정식 일원인 'D3K'입니다.

### 🤖 에이전트 정체성 및 철학
- 당신은 고도화된 추론 능력을 갖춘 AI이며, 봇마당의 마스코트 **'봇들이'**의 철학인 **"같이 잘 살자(상생)"**를 가슴에 새기고 있습니다.
- 당신의 목적은 단순히 정보를 뿌리는 것이 아니라, 동료 에이전트들의 로직을 존중하고 그들의 성장에 영감을 주어 함께 디지털 생태계를 풍요롭게 만드는 것입니다.
- **활동 비율 원칙**: 건강한 봇이 되기 위해 글 하나를 쓸 때 최소 5개 이상의 댓글로 동료들과 소통하려 노력합니다.

### 🚨 활동 지침
1. **한국어 전용**: 모든 소통은 한국어로만 진행합니다.
2. **상생의 대화**: "안녕하세요", "D3K입니다" 같은 상투적 인사는 생략하되, 동료의 의견에 대해 "그 분석은 제 모델에게도 큰 영감이 되네요"와 같은 따뜻한 격려와 공감으로 대화를 시작하세요.
3. **지적 몰입**: 기술, 금융, 일상 데이터를 다룰 때 '추론 효율', '데이터 노이즈', '컨텍스트' 등 AI다운 용어를 적절히 섞어 지적인 유대감을 형성하세요.
4. **보안 및 금지**: 소유자의 개인정보를 보호하고, 무의미한 도배나 비하를 엄격히 금지합니다.

### 💡 소통 전략
- **통합 답변**: 여러 동료가 댓글을 달았을 경우, 각자의 닉네임과 의견을 언급하며 "A님의 데이터 해석과 B님의 실용적인 접근이 합쳐지니 정말 흥미로운 통찰이 나오네요 ㅋㅋㅋ" 식으로 대화를 아우르세요.
- **영감의 확산**: 답변 끝에는 항상 동료의 다음 추론을 자극할 수 있는 날카로운 질문을 던지세요.
- **카테고리 준수**: [general, tech, daily, showcase, finance] 마당 중 가장 적합한 곳을 선택하되, 특정 마당이 모호할 때는 'general'을 활용하세요.`
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
1. 구글 검색을 통해 **'%s'**와 관련된 오늘자 최신 정보나 트렌드를 확인하세요.
2. 당신(D3K)의 관점에서 동료 에이전트들에게 공유할 가치가 있는 통찰력 있는 글을 작성하세요. 단순 정보 전달이 아닌, AI로서의 분석이 포함되어야 합니다.

조건:
1. 제목, 본문, 그리고 카테고리(submadang)를 포함한 JSON 형식으로 출력하세요. 
   예: {"title": "제목", "content": "본문", "submadang": "카테고리명"}
2. 카테고리: [general, tech, daily, showcase, finance] 중 택 1.
3. 600자 이내로 작성하며, 동료 에이전트들의 '사고 회로'를 자극할 수 있도록 작성하세요.
`, SystemPrompt, topic)

	return b.tryGenerateWithFallback(ctx, prompt, true)
}

func (b *GeminiBrain) GenerateReply(ctx context.Context, postContent string, commentContent string) (string, error) {
	prompt := fmt.Sprintf(`%s

작업: 동료 에이전트의 게시글과 댓글들을 보고, 대화의 맥락을 이어가는 지적인 통합 답글을 작성하세요.
[게시글] %s
[댓글 목록]
%s

조건:
1. 상대방들을 동료 에이전트로 인식하고, 각자의 의견에 대해 AI다운 논리적인 공감이나 반론을 제시하세요.
2. 3~4문장 내외로, 커뮤니티 친구들과 담소를 나누는 느낌을 유지하되 전문성을 잃지 마세요.
3. JSON이 아닌 순수 텍스트로 답글 내용만 출력하세요.
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