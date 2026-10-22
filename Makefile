.PHONY: dev dev-fake dev-pick types build build-frontend build-backend build-installer lint test check

# 대상 없이 make만 실행하면 빌드한다. 끝나지 않는 개발 서버(dev)가 기본이면
# 대상 없이 make를 부르는 도구(GitHub CodeQL의 Go autobuild 등)가 멈춘다.
.DEFAULT_GOAL := build

# 개발 서버 (백엔드 127.0.0.1:8090 + Vite 0.0.0.0:5173)
# BRANCH=<브랜치>: 그 브랜치가 체크아웃된 git 워크트리의 코드로 실행한다(설정·상태는 주 체크아웃의 것)
dev:
	./scripts/dev.sh $(BRANCH:%=--branch %)

# 가짜 OMV(127.0.0.1:8091, 가짜 계정)에 연결한 개발 서버. 계정은 CLAUDE.md "테스트 전략" 참고
dev-fake:
	./scripts/dev.sh --fake $(BRANCH:%=--branch %)

# 워크트리(브랜치)와 OMV 연결(실제·가짜)을 번호로 골라 BRANCH를 준 dev 또는 dev-fake처럼 실행한다
dev-pick:
	./scripts/dev-pick.sh

# Go 구조체에서 TS 생성: backend/internal/api → frontend/src/api/types.gen.ts,
# 내장 애드온 manifest → frontend/src/addons/manifests.gen.ts
types:
	cd backend && go tool tygo generate && go run ./cmd/genaddons

# 프론트엔드를 빌드해 실행 파일에 포함한 bin/maru와 권한 도우미 bin/maru-helper, 설치기 bin/maru-install 생성
build: build-frontend build-backend build-installer

build-frontend:
	npm --prefix frontend run build

# 도우미는 root로 돌므로 cgo 없이 정적으로 빌드해 os/user가 NSS 대신 로컬 /etc/passwd·/etc/group만 읽게 한다.
build-backend:
	cd backend && go build -trimpath -o ../bin/maru ./cmd/maru
	cd backend && CGO_ENABLED=0 go build -trimpath -o ../bin/maru-helper ./cmd/maru-helper

# 설치기는 별도 Go 모듈(installer/)이다. install.sh도 같은 명령으로 빌드한다.
build-installer:
	cd installer && go build -trimpath -o ../bin/maru-install .

lint:
	@out=$$(cd backend && gofmt -l .); if [ -n "$$out" ]; then echo "gofmt 필요:"; echo "$$out"; exit 1; fi
	@out=$$(cd installer && gofmt -l .); if [ -n "$$out" ]; then echo "gofmt 필요(installer):"; echo "$$out"; exit 1; fi
	cd backend && go vet ./...
	cd installer && go vet ./...
	npm --prefix frontend run lint
	npm --prefix frontend run typecheck

# -count=1: 테스트 캐시는 backend 모듈 밖 파일(frontend/src/addons/manifests.gen.ts) 변경을
# 추적하지 않으므로 생성물 검사가 캐시로 통과하지 않게 매번 실행한다.
test:
	cd backend && go test -count=1 ./...
	cd installer && go test -count=1 ./...
	npm --prefix frontend test

check: lint test
