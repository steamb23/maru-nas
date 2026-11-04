package format

import "maru/backend/internal/addons"

// SchemaID는 manifest 스키마의 $id다. 공개 저장소의 파일 주소와 같다.
const SchemaID = "https://raw.githubusercontent.com/steamb23/maru-nas/main/sdk/js/schema/manifest.schema.json"

// Schema는 manifest의 구조 스키마(JSON Schema 2020-12)다. SDK의 편집기 지원과 CLI(maru-addon validate)가 쓴다.
// 구조(필드, 형식, 길이, 범위)만 담는다. 필드 사이의 조건(쓰기는 조작이 있어야 함, 조작은 백엔드가 있어야 함,
// 최소 크기 ≤ 기본 크기)과 문구의 제어·서식 문자, 치환 문법, 대시보드 버전에 따른 SDK 수준은 Validate만 본다.
// addonpkg.Schema가 압축 파일 규칙(x-maru)을 더해 파일로 쓴다.
func Schema() map[string]any {
	str := func(pattern string, max int) map[string]any {
		s := map[string]any{"type": "string", "maxLength": max}
		if pattern != "" {
			s["pattern"] = pattern
		}
		return s
	}
	integer := func(min, max int) map[string]any {
		return map[string]any{"type": "integer", "minimum": min, "maximum": max}
	}
	object := func(required []string, props map[string]any) map[string]any {
		o := map[string]any{"type": "object", "additionalProperties": false, "properties": props}
		if len(required) > 0 {
			o["required"] = required
		}
		return o
	}
	size := object([]string{"width", "height"}, map[string]any{
		"width":  integer(1, MaxWindowSide),
		"height": integer(1, MaxWindowSide),
	})
	grid := object([]string{"cols", "rows"}, map[string]any{
		"cols": integer(1, addons.MaxWidgetSpan),
		"rows": integer(1, addons.MaxWidgetSpan),
	})
	entry := str(entryPattern.String(), 100)
	methods := map[string]any{"type": "array", "maxItems": MaxMethods, "uniqueItems": true, "items": str(addons.MethodPattern, 100)}

	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  SchemaID,
		"title":                "Maru NAS addon package manifest",
		"description":          "manifest.json of a Maru NAS addon package (.maru-addon). The dashboard checks more than this schema when installing.",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"format", "id", "version", "sdk", "name", "author", "icon", "views"},
		"properties": map[string]any{
			"$schema":     map[string]any{"type": "string"},
			"format":      map[string]any{"const": Version},
			"id":          str(addons.IDPattern, addons.MaxPackageIDLen),
			"version":     str(versionPattern.String(), MaxVersionLen),
			"sdk":         map[string]any{"type": "integer", "minimum": 1},
			"name":        textSchema(MaxNameLen),
			"description": textSchema(MaxDescriptionLen),
			"author": object([]string{"name"}, map[string]any{
				"name": nonBlank(MaxAuthorLen),
				"url":  str(`^https?://[^\s/?#]+`, MaxURLLen),
			}),
			"icon": map[string]any{"const": IconFile},
			"views": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"minProperties":        1,
				"properties": map[string]any{
					"window": object([]string{"entry", "defaultSize"}, map[string]any{
						"entry": entry, "defaultSize": size, "minSize": size,
					}),
					"widget": object([]string{"entry", "defaultSize"}, map[string]any{
						"entry": entry, "defaultSize": grid, "minSize": grid, "maxSize": grid,
					}),
				},
			},
			"requires": object(nil, map[string]any{
				"admin": map[string]any{"type": "boolean"},
				"plugins": map[string]any{
					"type": "array",
					"items": object([]string{"package", "rpcFile"}, map[string]any{
						"package": str(pluginPattern.String(), 100),
						"rpcFile": str(rpcFilePattern.String(), 100),
					}),
				},
			}),
			"rpc": object(nil, map[string]any{"read": methods, "write": methods}),
			"actions": map[string]any{
				"type":     "array",
				"maxItems": MaxActions,
				"items": object([]string{"name"}, map[string]any{
					"name":           str(addons.IDPattern, 32),
					"admin":          map[string]any{"type": "boolean"},
					"confirm":        map[string]any{"type": "boolean"},
					"timeoutSeconds": integer(0, MaxActionTimeout),
					"label":          textSchema(MaxLabelLen),
					"confirmText":    textSchema(MaxConfirmLen),
				}),
			},
			"backend": object([]string{"network"}, map[string]any{
				"network": map[string]any{
					"type":        "array",
					"uniqueItems": true,
					"items":       map[string]any{"enum": []string{NetworkLAN, NetworkLoopback}},
				},
			}),
		},
	}
}

// textSchema는 언어별 문구다. 키는 지원 언어이고 en이 있어야 한다.
func textSchema(max int) map[string]any {
	return map[string]any{
		"type":                 "object",
		"required":             []string{FallbackLanguage},
		"propertyNames":        map[string]any{"enum": Languages},
		"additionalProperties": nonBlank(max),
	}
}

// nonBlank는 공백만이 아닌 문자열이다.
func nonBlank(max int) map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": max, "pattern": `\S`}
}
