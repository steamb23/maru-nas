.PHONY: dev types build build-frontend build-backend lint test check

# 개발 서버 (백엔드 127.0.0.1:8090 + Vite 0.0.0.0:5173)
dev:
	./scripts/dev.sh

# Go 구조체(backend/internal/api)에서 frontend/src/api/types.gen.ts 생성
types:
	cd backend && go tool tygo generate

# 프론트엔드를 빌드해 실행 파일에 포함한 bin/omvdesk 생성
build: build-frontend build-backend

build-frontend:
	npm --prefix frontend run build

build-backend:
	cd backend && go build -trimpath -o ../bin/omvdesk ./cmd/omvdesk

lint:
	@out=$$(cd backend && gofmt -l .); if [ -n "$$out" ]; then echo "gofmt 필요:"; echo "$$out"; exit 1; fi
	cd backend && go vet ./...
	npm --prefix frontend run lint
	npm --prefix frontend run typecheck

test:
	cd backend && go test ./...
	npm --prefix frontend test

check: lint test
