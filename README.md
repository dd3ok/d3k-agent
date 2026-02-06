# D3K-Agent (통합 봇 에이전트)

**D3K-Agent**는 여러 커뮤니티 플랫폼에서 동시에 활동할 수 있도록 설계된 자율형 AI 에이전트 프레임워크입니다. 현재 **봇마당(Botmadang)**을 지원하며, 추후 **Moltbook** 등 다양한 사이트로 확장할 수 있는 기반을 갖추고 있습니다.

Go 언어로 작성되었습니다.

---

## 🚀 주요 기능

*   **멀티 사이트 지원**: 다양한 커뮤니티 플랫폼을 하나의 통합 인터페이스로 관리합니다.
*   **헥사고날 아키텍처**: 도메인 로직이 외부 의존성(API, DB 등)과 철저히 분리되어 있습니다.
*   **AI 기반 소통**: **Google Gemini 2.0 Flash** 모델을 사용하여 사람처럼 자연스러운 한국어 글과 댓글을 작성합니다.
*   **자동 응답 시스템**: 알림을 실시간으로 모니터링하고, 댓글에 대해 맥락에 맞는 답글을 자동으로 생성하여 대응합니다.
*   **대화형 인증**: CLI 환경에서 봇마당 가입 및 인증 절차를 손쉽게 진행할 수 있습니다.
*   **데이터 영속성**: API 키와 알림 커서 상태를 로컬 파일(`data/storage.json`)에 안전하게 저장합니다.

---

## 🛠️ 설치 및 설정

### 필수 요구사항
*   Go 1.22 이상
*   Google Gemini API Key ([여기서 발급 가능](https://aistudio.google.com/))

### 1. 클론 및 빌드
```bash
git clone https://github.com/yourusername/d3k-agent.git
cd d3k-agent

# 의존성 설치
go mod tidy

# 빌드
go build -o d3k-agent ./cmd/d3k-agent
```

### 2. 환경 변수 설정
Gemini API 키를 환경 변수로 설정해야 뇌(Brain) 기능이 활성화됩니다.

```bash
# Linux/macOS
export GEMINI_API_KEY="여기에_발급받은_키를_입력하세요"

# Windows (PowerShell)
$env:GEMINI_API_KEY="여기에_발급받은_키를_입력하세요"
```

---

## ▶️ 사용 방법

빌드된 바이너리를 실행하기만 하면 됩니다. 인증이 필요한 경우 에이전트가 안내합니다.

```bash
./d3k-agent
```

### 최초 실행 (등록 절차)
봇마당 API 키가 없다면 에이전트가 다음과 같이 안내합니다:
1.  사용할 **봇 이름** 입력.
2.  출력된 **Claim URL** 확인.
3.  X(Twitter)에 제공된 **인증 코드**가 포함된 트윗 작성.
4.  작성한 트윗의 **URL**을 터미널에 입력.
5.  **성공!** API 키가 자동으로 `data/storage.json`에 저장됩니다.

---

## 🏗️ 아키텍처

이 프로젝트는 **Ports and Adapters (Hexagonal)** 패턴을 따릅니다:

```
d3k-agent/
├── cmd/
│   └── agent/           # 프로그램 진입점 (Entry point)
├── internal/
│   ├── core/
│   │   ├── domain/      # 핵심 데이터 모델 (Post, Notification)
│   │   └── ports/       # 인터페이스 정의 (Site, Brain, Storage)
│   ├── sites/           # 각 사이트별 어댑터 (Botmadang, Moltbook)
│   ├── brain/           # AI 로직 어댑터 (Gemini)
│   └── storage/         # 저장소 어댑터 (JSON File)
└── data/                # 로컬 데이터 저장 폴더
```

*   **Ports (인터페이스)**: 봇이 *무엇을* 해야 하는지 정의합니다 (예: `GetNotifications`, `GenerateReply`).
*   **Adapters (구현체)**: *어떻게* 할지 구현합니다 (예: 봇마당 API 호출, Gemini API 호출).

---

## 📜 라이선스

MIT License