.PHONY: dev dev-fake types build build-frontend build-backend lint test check

# 개발 서버 (백엔드 127.0.0.1:8090 + Vite 0.0.0.0:5173)
# BRANCH=<브랜치>: 그 브랜치가 체크아웃된 git 워크트리의 코드로 실행한다(설정·상태는 주 체크아웃의 것)
dev:
	./scripts/dev.sh $(BRANCH:%=--branch %)

# 가짜 OMV(127.0.0.1:8091, 가짜 계정)에 연결한 개발 서버. 계정은 CLAUDE.md "테스트 전략" 참고
dev-fake:
	./scripts/dev.sh --fake $(BRANCH:%=--branch %)

# Go 구조체에서 TS 생성: backend/internal/api → frontend/src/api/types.gen.ts,
# 내장 애드온 manifest → frontend/src/addons/manifests.gen.ts
types:
	cd backend && go tool tygo generate && go run ./cmd/genaddons

# 프론트엔드를 빌드해 실행 파일에 포함한 bin/onmaru 생성
build: build-frontend build-backend

build-frontend:
	npm --prefix frontend run build

build-backend:
	cd backend && go build -trimpath -o ../bin/onmaru ./cmd/onmaru

lint:
	@out=$$(cd backend && gofmt -l .); if [ -n "$$out" ]; then echo "gofmt 필요:"; echo "$$out"; exit 1; fi
	cd backend && go vet ./...
	npm --prefix frontend run lint
	npm --prefix frontend run typecheck

# -count=1: 테스트 캐시는 backend 모듈 밖 파일(frontend/src/addons/manifests.gen.ts) 변경을
# 추적하지 않으므로 생성물 검사가 캐시로 통과하지 않게 매번 실행한다.
test:
	cd backend && go test -count=1 ./...
	npm --prefix frontend test

check: lint test
