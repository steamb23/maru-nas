// genaddons는 내장 애드온 manifest로 프론트 목록(manifests.gen.ts)을 생성한다. `make types`가 실행한다.
package main

import (
	"flag"
	"log"
	"os"

	"omv-desktop-dashboard/backend/internal/addons/builtin"
	"omv-desktop-dashboard/backend/internal/addons/sysinfo"
)

func main() {
	out := flag.String("o", "../frontend/src/addons/manifests.gen.ts", "출력 파일")
	flag.Parse()

	src, err := builtin.New(builtin.Deps{System: sysinfo.NewReader()}).GenerateTS()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		log.Fatal(err)
	}
}
