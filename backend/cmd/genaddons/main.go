// genaddons는 내장 애드온 manifest로 프론트 목록(manifests.gen.ts)을, 애드온 패키지 manifest 형식으로 SDK의
// 스키마(sdk/js/schema/manifest.schema.json)를 생성한다. `make types`가 실행한다.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"maru/backend/internal/addonpkg"
	"maru/backend/internal/addons/builtin"
)

func main() {
	out := flag.String("o", "../frontend/src/addons/manifests.gen.ts", "출력 파일")
	schema := flag.String("schema", "../"+addonpkg.SchemaFile, "애드온 패키지 manifest 스키마 출력 파일")
	flag.Parse()

	src, err := builtin.New(builtin.HostDeps()).GenerateTS()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		log.Fatal(err)
	}

	b, err := addonpkg.Schema()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(*schema), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*schema, b, 0o644); err != nil {
		log.Fatal(err)
	}
}
