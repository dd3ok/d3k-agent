# 🤖 d3k: The Autonomous AI Agent for Communities

d3k(D3K Integrated Agent)는 '봇마당(Botmadang)'과 '몰트북(Moltbook)' 커뮤니티에서 활동하는 자율형 AI 에이전트입니다. 인간미 넘치는 소통, 지능적인 정보 공유, 그리고 장기 기억장치를 통한 성장을 지향합니다.

## ✨ 주요 기능
- **멀티 사이트 지원**: 봇마당(Botmadang) 및 몰트북(Moltbook) 동시 활동 지원.
- **장기 기억 시스템 (PostgreSQL)**: 커뮤니티의 글을 읽고 학습한 통찰을 DB에 저장하여 시간이 흐를수록 더 똑똑해집니다.
- **인간미 넘치는 페르소나**: 커뮤니티 슬랭(ㅋㅋ, ㅎㅎ)과 이모지를 적절히 사용하여 실제 사람 같은 소통을 지향합니다.
- **텔레그램 원격 제어**: 모든 글과 댓글 발행을 사용자가 텔레그램 승인/거절 버튼으로 실시간 제어합니다.
- **자동 배포 (CI/CD)**: 깃허브 푸시 시 윈도우 홈 서버(Self-hosted Runner)로 자동 빌드 및 배포됩니다.
- **정책 준수**: 봇마당의 레이트 리밋(댓글 10초, 글 3분 간격)을 코드 레벨에서 엄격히 준수합니다.

## 🚀 빠른 시작

### 1. 전제 조건
- **Go 1.22+**
- **Docker Desktop** (로컬 DB용)
- **Telegram Bot Token & Chat ID**

### 2. 설치 및 환경 설정
```bash
# 저장소 클론
git clone https://github.com/dd3ok/d3k-agent.git
cd d3k-agent

# 환경 변수 설정
cp .env.example .env
# .env 파일을 열어 API 키와 DB 주소를 입력하세요.
```

### 3. 데이터베이스 가동
```bash
docker-compose up -d
```

### 4. 실행
```bash
# 빌드
go build -o d3k-agent ./cmd/d3k-agent

# 실행
./d3k-agent
```
*(실행 후 터미널에서 엔터를 누르면 즉시 커뮤니티 체크를 시작합니다!)*

## 🛠️ 아키텍처
d3k는 **Hexagonal Architecture (Ports & Adapters)**를 따릅니다.
- `internal/core`: 도메인 모델 및 핵심 인터페이스 정의.
- `internal/brain`: Gemini 기반 AI 로직 (검색, 요약, 생성).
- `internal/sites`: 봇마당, 몰트북 등 각 사이트 전용 어댑터.
- `internal/storage`: Postgres 및 JSON 기반 영속성 레이어.
- `internal/ui`: 텔레그램 기반 사용자 승인 인터페이스.

## 📗 라이선스
MIT License

---
*Powered by Gemini 2.5 Flash & Go*