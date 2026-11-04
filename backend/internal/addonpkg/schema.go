package addonpkg

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"

	"maru/backend/internal/addonpkg/format"
	"maru/backend/internal/api"
)

// SchemaFile은 manifest 스키마 파일의 저장소 루트 기준 경로다. `make types`(genaddons)가 쓰고 테스트가 비교한다.
// npm 패키지(@maru-nas/addon-sdk)에 함께 들어가도록 sdk/js 아래에 둔다.
const SchemaFile = "sdk/js/schema/manifest.schema.json"

// Schema는 manifest 스키마(format.Schema)에 압축 파일 규칙(x-maru)을 더한 파일 내용이다. SDK CLI
// (maru-addon validate, pack)는 x-maru를 읽어 이 파일의 규칙(Read, ReadDir)을 따른다. 상수를 CLI에 다시 적지 않는다.
func Schema() ([]byte, error) {
	s := format.Schema()
	s["x-maru"] = map[string]any{
		"sdk": api.AddonSDKLevel,
		"archive": map[string]any{
			"limits": map[string]any{
				"compressed": DefaultLimits.Compressed,
				"total":      DefaultLimits.Total,
				"entries":    DefaultLimits.Entries,
				"uiFile":     DefaultLimits.UIFile,
				"backend":    DefaultLimits.Backend,
				"i18n":       DefaultLimits.I18n,
				"icon":       DefaultLimits.Icon,
				"text":       DefaultLimits.Text,
				"manifest":   format.MaxManifestBytes,
			},
			"maxDepth":       maxDepth,
			"segmentPattern": segmentPattern.String(),
			"reservedUI":     reservedUI,
			"topFiles":       []string{"manifest.json", format.IconFile, "LICENSE", "README.md"},
			"topDirs":        []string{"ui", "i18n", "backend"},
			"backendFiles":   slices.Sorted(maps.Keys(backendArchs)),
			// dev/는 개발 디렉터리에서만 쓰는 파일(가짜 OMV 응답 등)이다. 묶을 때 뺀다.
			"devDir": "dev",
		},
		"i18n": map[string]any{
			"languages":     format.Languages,
			"fallback":      format.FallbackLanguage,
			"maxKeys":       maxI18nKeys,
			"keyPattern":    i18nKeyPattern.String(),
			"maxValueBytes": maxI18nValueLen,
		},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
