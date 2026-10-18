// Package setup은 온마루 설치기다. 설정 레지스트리와 설치·제거 단계로 install.sh가 하던 일을 한다.
package setup

import (
	"fmt"
	"os"
	"path/filepath"
)

// settings는 설치 설정 목록이다. 설정 정의는 다음 단계에서 더한다.
var settings = Registry{}

// Main은 설치기를 실행하고 종료 코드를 돌려준다. 지금은 명령줄과 사용법만 다룬다.
func Main(args []string) int {
	lang := DetectLang(os.Getenv)
	opts, _, err := parseArgs(settings, args)
	if opts.Lang != "" {
		lang = opts.Lang
	}
	name := "./install.sh"
	if exe, e := os.Executable(); e == nil {
		name = filepath.Join(filepath.Dir(filepath.Dir(exe)), "install.sh")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n\n%s", err, usage(settings, lang, name))
		return 2
	}
	if opts.Help {
		fmt.Print(usage(settings, lang, name))
		return 0
	}
	fmt.Fprintln(os.Stderr, "error: not implemented yet")
	return 1
}
