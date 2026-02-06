# D3K-Agent 프로젝트 점검 요약

## 범위
- 로컬 문서: `README.md`, `AGENT_GUIDE.md`, `DESIGN.md`, `API_DOCS.md`, `OPENAPI_SPEC.md`
- 코드: `cmd/d3k-agent/main.go`, `internal/*`
- 문서/URL: botmadang.org 공식 문서 및 OpenAPI

## 핵심 결론
- 문서에 명시된 정책은 코드에서 강제되지 않는 부분이 많아 정책 준수 리스크가 있습니다.
- 아키텍처는 포트/어댑터 방향성은 맞으나 유스케이스 로직이 `main`에 집중되어 있습니다.
- 운영/보안 리스크로는 HTTP 타임아웃 미설정, 응답 코드 미검증, 스토리지 락 오류 등이 있습니다.

## 정책/규정 적합성
- 문서 기준 정책
  - 한국어 필수
  - 스팸 금지 / 중복 댓글 금지
  - API 키 외부 공개 금지
  - 레이트리밋: 글 3분당 1, 댓글 10초당 1, API 분당 100회
- 코드 반영 현황
  - 한국어 필수/중복 댓글 금지/스팸 방지 규칙의 코드 강제 없음
  - 레이트리밋은 일일 횟수 제한(글 4, 댓글 12)만 구현
  - API 분당 100회 제한 로직 없음
  - 시스템 프롬프트에 카테고리 제한이 있으나 최신 마당 확장 반영 불충분 가능

## 아키텍처 점검
- 장점
  - 포트/어댑터 인터페이스 분리 (`internal/core/ports`)
  - 어댑터별 구현 분리 (`internal/sites/*`)
- 리스크
  - `DESIGN.md`에 명시된 `internal/app` 유스케이스 레이어 실제 부재
  - `cmd/d3k-agent/main.go`에 핵심 정책/승인/포스팅 로직 집중
  - 멀티 사이트는 확장 가능하나 `moltbook`은 placeholder 수준

## 보안/운영 리스크
1. HTTP 타임아웃 미설정
   - `http.Client{}` 기본값 사용으로 무기한 대기 가능
2. API 응답 코드 미검증
   - POST/GET 요청의 `StatusCode` 체크가 부족
3. 스토리지 락 오류
   - `JSONStorage.load()`가 `RLock()` 상태에서 `s.Data` 변경
4. Telegram Markdown 파싱/인젝션 위험
   - Markdown 모드에서 사용자/모델 텍스트 이스케이프 없음
5. 레이트리밋 정책 미준수 가능성
   - 댓글/게시글 간 시간 간격 제한 미구현

## 개선 우선순위 제안
1. 안정성/보안
   - HTTP 타임아웃 설정
   - 모든 API 응답 코드 검증 및 에러 처리
   - 스토리지 락 수정
   - Telegram 메시지 이스케이프 처리
2. 정책 준수
   - 레이트리밋(댓글 10초, 글 3분, 분당 100회) 강제
   - 한국어 필수/중복 댓글 금지 검증 추가
3. 아키텍처
   - 유스케이스 로직을 `internal/app` 계층으로 분리

## 참고 파일
- `README.md`
- `AGENT_GUIDE.md`
- `DESIGN.md`
- `API_DOCS.md`
- `OPENAPI_SPEC.md`
- `cmd/d3k-agent/main.go`
- `internal/brain/gemini_client.go`
- `internal/sites/botmadang/client.go`
- `internal/storage/json_storage.go`
- `internal/ui/telegram/bot.go`
