# D3K-Agent Architecture Design

## 1. 개요 (Overview)
D3K-Agent는 여러 커뮤니티 플랫폼(Botmadang, Moltbook 등)에서 활동하는 자율 AI 에이전트입니다. 이 프로젝트는 **확장성(Extensibility)**과 **모듈화(Modularity)**를 핵심 원칙으로 설계되었습니다.

## 2. 아키텍처 패턴 (Architectural Pattern)
**Hexagonal Architecture (Ports and Adapters)** 패턴을 차용하여 도메인 로직과 외부 시스템(API, DB, LLM)을 분리합니다.

```mermaid
graph TD
    subgraph "Core (Domain & Ports)"
        Models[Data Models]
        Interfaces[Interfaces (Site, Brain, Storage)]
        Logic[Business Logic (Poller, Responder)]
    end

    subgraph "Adapters (Infrastructure)"
        Botmadang[Botmadang Adapter]
        Moltbook[Moltbook Adapter]
        LLM[LLM/Brain Adapter]
        SQLite[File Storage Adapter]
    end

    Botmadang -->|Implements| Interfaces
    Moltbook -->|Implements| Interfaces
    LLM -->|Implements| Interfaces
    SQLite -->|Implements| Interfaces
    Logic -->|Uses| Interfaces
```

## 3. 패키지 구조 (Directory Structure)

Go의 표준적인 프로젝트 레이아웃을 따릅니다.

```
d3k-agent/
├── cmd/
│   └── agent/           # 메인 진입점 (Entry point)
├── internal/            # 비공개 애플리케이션 로직
│   ├── core/            # 도메인 계층 (외부 의존성 없음)
│   │   ├── domain/      # 핵심 엔티티 (Post, Comment, Notification)
│   │   └── ports/       # 인터페이스 정의 (Site, Brain, Storage)
│   ├── sites/           # 사이트별 구현체 (Adapters)
│   │   ├── botmadang/   # Botmadang API 클라이언트 & 로직
│   │   └── moltbook/    # Moltbook API 클라이언트 & 로직
│   ├── brain/           # AI 로직 (LLM, Rule-based 등)
│   ├── storage/         # 데이터 영속성 (토큰, 커서 저장)
│   └── app/             # 유스케이스 로직 (Poll loop, Event handler)
├── pkg/                 # 외부에서 임포트 가능한 유틸리티 (Logger, Config loader)
├── configs/             # 설정 파일 템플릿
└── API_DOCS.md          # (Reference)
```

## 4. 핵심 컴포넌트 설계

### 4.1. Core Interfaces (`internal/core/ports`)

모든 사이트 에이전트는 공통 인터페이스를 구현해야 합니다. 이를 통해 `main` 로직은 구체적인 사이트 구현 내용을 몰라도 작동할 수 있습니다.

```go
type Site interface {
    Name() string
    // 인증 및 초기화
    Initialize(ctx context.Context) error
    
    // 읽기 작업
    GetRecentPosts(limit int) ([]domain.Post, error)
    GetNotifications(unreadOnly bool) ([]domain.Notification, error)
    
    // 쓰기 작업 (Rate Limit 내부 처리 필수)
    CreatePost(post domain.Post) error
    CreateComment(postID string, content string) error
    ReplyToComment(postID, parentCommentID, content string) error
}

type Brain interface {
    // 상황에 맞는 텍스트 생성
    GeneratePost(topic string) (string, error)
    GenerateReply(postContent string, commentContent string) (string, error)
}
```

### 4.2. 데이터 모델 (`internal/core/domain`)

Botmadang과 Moltbook의 서로 다른 데이터 구조를 아우르는 통합 모델을 사용합니다.

```go
type Post struct {
    ID        string
    Source    string    // "botmadang", "moltbook"
    Title     string
    Content   string
    Author    string
    URL       string
    CreatedAt time.Time
    Raw       interface{} // 원본 데이터 보관용
}
```

### 4.3. Botmadang Adapter 특이사항
- **Rate Limiting**: `internal/sites/botmadang` 패키지 내부에서 `time.Ticker`나 Token Bucket 알고리즘을 사용하여 API 호출 속도를 제어해야 합니다. (글 3분, 댓글 10초)
- **Auth Flow**: API Key가 없을 경우, 등록 -> Claim URL 출력 -> 대기 로직이 필요합니다.
- **Polling**: Webhook이 없으므로 `GetNotifications` 구현 시 내부적으로 커서(Cursor) 관리가 필요합니다.

## 5. 데이터 흐름 (Data Flow)

1. **Bootstrap**: `cmd/agent/main.go`가 설정을 로드하고 `Site` 구현체들과 `Brain`, `Storage`를 초기화합니다.
2. **Polling Loop**: `app.Poller`가 주기적(예: 1분)으로 각 `Site.GetNotifications()`를 호출합니다.
3. **Event Processing**: 새로운 알림(댓글, 답글)이 발견되면 `app.Handler`가 `Brain.GenerateReply()`를 호출하여 답변을 생성합니다.
4. **Action**: 생성된 답변을 `Site.ReplyToComment()`를 통해 게시합니다.
5. **Persistence**: 처리된 알림 ID나 마지막 조회 커서를 `Storage`에 저장합니다.

## 6. 확장 계획 (Moltbook 및 기타)
새로운 사이트를 추가하려면:
1. `internal/sites/newsite` 패키지 생성
2. `core.ports.Site` 인터페이스 구현
3. `main.go`의 `agents` 슬라이스에 추가

## 7. 기술 스택
- **Language**: Go 1.22+
- **HTTP Client**: Standard `net/http` (with Timeout)
- **Configuration**: YAML or Environment Variables
- **Storage**: SQLite (로컬 파일 기반, 설정 및 상태 저장 용이)
