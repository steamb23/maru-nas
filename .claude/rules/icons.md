---
paths:
  - "frontend/src/**"
  - "vendor/icons/**"
---

# 아이콘

- 아이콘 테마는 **Colloid**(vinceliuice/Colloid-icon-theme, GPL-3.0) 하나만 쓴다. 다른 테마와 섞지 않는다
- 원본은 git submodule(shallow)로 `vendor/icons/colloid/`에 두고, 테마의 `LICENSE`와 출처를 함께 보관한다
- 아이콘은 경로를 직접 쓰지 말고 리졸버(`getIcon(name)`, `src/icons/resolver.ts`)를 통해 freedesktop 아이콘 이름(`folder-music`, `drive-harddisk` 등)으로 가져온다. 테마 교체는 리졸버 설정만 바꾸면 되게 한다
- 매핑 테이블에 `?url` import로 한 줄씩 추가한 아이콘만 번들에 들어간다. 디렉터리 glob import는 쓰지 않는다
- 테마별로 이름이 다른 아이콘(창 최소화 등)은 리졸버의 매핑 테이블에서 처리한다
- 컬러 아이콘(폴더·앱·파일 형식)은 `<img>`로 그대로, 단색 `-symbolic` 아이콘은 CSS `mask-image` 또는 인라인 SVG + `fill: currentColor`로 색을 입힌다(`IconImage`). `<img>`로 넣으면 색을 바꿀 수 없다
- 테마에 없는 대시보드 전용 아이콘은 Colloid 폴더·앱 SVG 위에 MIT 라이선스 라인 아이콘(Tabler, Lucide 등)을 얹어 합성한다
- 브랜드 앱 아이콘(Discord, Steam 등)은 실제 그 앱을 가리킬 때만 쓴다. 장식이나 대표 이미지로 쓰지 않는다
