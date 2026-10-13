# 아이콘 원본

## colloid/

- 이름: Colloid icon theme
- 출처: https://github.com/vinceliuice/Colloid-icon-theme
- 라이선스: GPL-3.0 (`colloid/LICENSE`)
- 형태: git submodule (shallow). 고정 커밋은 `git submodule status`로 확인한다
  - 추가 시점: `ceac6608ecd0e40025cbc2ebbd32bf0e0f4ebc6a` (2026-08-29)

`colloid/src/` 아래 SVG를 직접 참조한다. 설치 스크립트(`install.sh`)가 만드는 색상 변형은 쓰지 않는다.
아이콘은 경로가 아니라 `frontend/src/icons/resolver.ts`의 `getIcon(name)`으로 가져온다.

클론 후 서브모듈 받기:

```sh
git submodule update --init --depth 1
```
