# 아이콘

- 기본 아이콘 테마는 **Colloid**(vinceliuice/Colloid-icon-theme, GPL-3.0)다. 일반 아이콘(동작, 상태, 앱, 장치, 폴더 등)은 Colloid만 쓰고 다른 아이콘 테마와 섞지 않는다
- 원본은 출처마다 `vendor/icons/<출처>/`에 둔다. Colloid는 git submodule(shallow)이고 테마의 `LICENSE`와 출처를 함께 보관한다. 출처 목록은 `vendor/icons/README.md`(출처마다 `## <폴더>/` 절)
- 아이콘은 경로를 직접 쓰지 말고 리졸버(`getIcon(name)`, `src/icons/resolver.ts`)를 통해 freedesktop 아이콘 이름(`folder-music`, `drive-harddisk` 등)으로 가져온다. 테마 교체는 출처 테이블만 바꾸면 되게 한다
- 리졸버는 출처별 테이블(`src/icons/sources/<출처>.ts`, `ICON_SOURCES`)을 합친다. 테이블에 `?url` import로 한 줄씩 추가한 아이콘만 번들에 들어간다. 디렉터리 glob import는 쓰지 않는다
  - import 경로: Colloid는 `@colloid/…`(`colloid/src/`), 그 밖의 출처는 `@vendor-icons/<출처>/…`
  - `resolver.test.ts`가 출처 사이 이름 중복, README 절, 출처 폴더의 `LICENSE`·`NOTICE`를 검사한다
- 테마별로 이름이 다른 아이콘(창 최소화 등)은 리졸버의 매핑 테이블에서 처리한다
- 브랜드 앱 아이콘(Colloid의 Discord, Steam 등과 제3자 로고)은 실제 그 앱을 가리킬 때만 쓴다. 장식이나 대표 이미지로 쓰지 않는다
- 앱을 나타내는 아이콘(`getApp(id).icon`: "모든 앱" 타일, 바탕화면 바로가기, 작업표시줄, 제목 표시줄)은 `IconImage`에 `app`을 준다. 메뉴·위젯 머리글 등 다른 자리에서는 주지 않는다(`appRadius`가 이때만 적용된다)
- 컬러 아이콘(폴더·앱·파일 형식, 브랜드 로고)은 `<img>`로 그대로, 단색 `-symbolic` 아이콘은 CSS `mask-image` 또는 인라인 SVG + `fill: currentColor`로 색을 입힌다(`IconImage`). `<img>`로 넣으면 색을 바꿀 수 없다

## 온마루 로고

- 온마루 자신의 로고는 테마 아이콘도 제3자 에셋도 아니므로 리졸버와 `vendor/icons/`에 넣지 않고 `frontend/src/assets/logo/`에 둔다. 쓰는 곳에서 `?url`로 직접 import한다
  - `onmaru.svg`(둥근 타일): 실제로 쓰는 로고. favicon(`index.html`)
  - `onmaru-square.svg`(각진 타일): 그림은 같고 타일 모서리만 각지다. 각진 자리가 필요할 때를 위해 보관한다. 두 파일은 한 문서에 함께 넣어도 겹치지 않게 id 접두사가 다르다
  - `onmaru-lockup.svg`(묶음 로고): 로고 오른쪽에 "온마루"(온은 Light에 파랑, 마루는 Bold)와 그 아래 "Onmaru"를 둔다. README 머리와 로그인 화면 제목에 쓴다. 글꼴은 Pretendard(SIL OFL 1.1)이고 글자를 윤곽선 path로 바꿔 넣었으므로 글꼴 파일은 저장소에 두지 않는다. 글자색은 `prefers-color-scheme`을 따른다. 고칠 때는 글꼴에서 윤곽선을 다시 뽑아 같은 배치(파일 주석)로 만든다
- 64칸 격자에 4칸 단위로 그렸다(NAS 디스크 베이 3단이 16px에서도 한 픽셀 줄로 갈린다). 그래서 16px의 배수(16·32·48·64·80…)로 그려야 선이 정수 픽셀에 맞는다

## 제3자 에셋

Colloid에 없는 아이콘만 아래 두 경우에 한해 들인다.

1. **브랜드 로고**: 테마에 없는 제품의 로고(예: OMV 워크벤치 앱의 openmediavault 로고). 원본 파일을 수정하지 않고 색을 바꾸지 않는다(`color`). 테마 아이콘과 맞추는 조정은 그릴 때만 테이블 값으로 한다: 크기는 `inset`(여백), 각진 로고의 모서리는 `appRadius`(앱 아이콘 자리에서만 둥글게 깎음)
2. **대시보드 전용 아이콘**: Colloid 폴더·앱 SVG 위에 MIT 라이선스 라인 아이콘(Tabler, Lucide 등)을 얹어 합성한다

들일 때 지킬 것:

- 라이선스가 GPL-3.0과 호환되는지 먼저 확인한다. 상표 정책이 있으면 따른다
- `vendor/icons/<출처>/`에 원본을 수정하지 않고 두고, `LICENSE` 또는 `NOTICE`에 저작권자·라이선스·출처 URL·버전(커밋)·sha256을 적는다
- `vendor/icons/README.md`에 절을 추가하고, `src/icons/sources/<출처>.ts` 테이블을 만들어 `resolver.ts`의 `ICON_SOURCES`와 합친 테이블에 넣는다
