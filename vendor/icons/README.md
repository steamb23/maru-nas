# 아이콘 원본

대시보드 아이콘의 출처 목록이다. 출처마다 이 디렉터리 아래에 폴더 하나를 두고, 아래에 같은 이름의 절을 둔다.
프론트엔드는 출처마다 `frontend/src/icons/sources/<폴더>.ts` 테이블을 두고 `resolver.ts`가 합친다
(`getIcon(name)`). 규칙은 `.claude/rules/icons.md`에 있다.

- 기본 출처는 Colloid 테마다. 일반 아이콘(동작, 상태, 앱, 장치 등)은 Colloid만 쓴다
- 테마 밖 에셋은 Colloid에 없을 때만 들인다. GPL-3.0과 호환되는 라이선스여야 하며, 원본을 수정하지 않고
  출처·버전·해시·라이선스를 기록한다(폴더 안 `LICENSE` 또는 `NOTICE`)
- 브랜드 로고는 실제 그 제품을 가리킬 때만 쓴다

## colloid/

- 이름: Colloid icon theme
- 출처: https://github.com/vinceliuice/Colloid-icon-theme
- 라이선스: GPL-3.0 (`colloid/LICENSE`)
- 형태: git submodule (shallow). 고정 커밋은 `git submodule status`로 확인한다
  - 추가 시점: `ceac6608ecd0e40025cbc2ebbd32bf0e0f4ebc6a` (2026-08-29)
- 참조: `@colloid` alias(`colloid/src/`)로 SVG를 직접 import한다. 설치 스크립트(`install.sh`)가 만드는 색상 변형은 쓰지 않는다
- 합성: Colloid 파일만으로 만든 합성 아이콘은 이 폴더가 아니라 `frontend/src/icons/composed/`에 둔다. 재료와 커밋은 각 파일 첫머리 주석에 있다

클론 후 서브모듈 받기:

```sh
git submodule update --init --depth 1
```

## openmediavault/

- 이름: openmediavault 로고(`favicon.svg`)
- 출처: https://github.com/openmediavault/openmediavault 의 `deb/openmediavault/workbench/src/favicon.svg`
  - 커밋: `bc3a0ce88b6b450ad7390561565125b87b11246f`(이 파일의 마지막 변경). openmediavault 8.5.11-1 설치본과 같다
  - sha256: `8b34b5e4eb5330a02971b48e92b06885222da9e9a650cddb6381f41f99287069`
- 라이선스: GPL-3.0, 저작권 Volker Theile (`openmediavault/NOTICE`)
- 형태: 파일 복사본(수정하지 않음)
- 사용 제한: OMV 자신을 가리킬 때만 쓴다(OMV 워크벤치 앱). openmediavault는 상표·로고 사용 정책을 공개하지 않았다
- 참조: `@vendor-icons/openmediavault/favicon.svg`
