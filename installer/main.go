// maru-install은 마루 NAS 설치기다. 저장소의 install.sh가 이 프로그램을 빌드해(bin/maru-install) 실행한다.
// 사용법은 ./install.sh --help, 구조는 .claude/rules/installer.md에 있다.
package main

import (
	"os"

	"maru/installer/internal/setup"
)

func main() {
	os.Exit(setup.Main(os.Args[1:]))
}
