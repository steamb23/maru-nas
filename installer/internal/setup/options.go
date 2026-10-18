package setup

import (
	"fmt"
	"strings"
)

// Action은 설치기가 할 일이다.
type Action int

const (
	// ActionInstall은 설정을 묻고 빌드한 뒤 설치한다(1단계 + 2단계).
	ActionInstall Action = iota
	// ActionServiceInstall은 설치 단계(2단계)만 한다(root).
	ActionServiceInstall
	// ActionUninstall은 제거한다.
	ActionUninstall
)

// Options는 설정이 아닌 명령줄 옵션이다.
type Options struct {
	Action Action
	// Upgrade는 --upgrade(기존 설치가 있어야 하고 확인 없이 진행), Reinstall은 --reinstall(모든 설정을 다시 묻는다)이다.
	// 둘 다 없으면 기존 설치가 있는지로 정한다(resolveMode).
	Upgrade   bool
	Reinstall bool
	NoBuild   bool
	DryRun    bool
	Plain     bool
	Help      bool
	// Lang은 --lang 값이다. 비면 로캘로 고른다.
	Lang string
}

// usageError는 잘못된 명령줄이다. 사용법을 보여 주고 2로 끝낸다.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// parseArgs는 명령줄을 읽는다. 설정 옵션은 레지스트리에서 나온다. --name=value와 --name value를 모두 받는다.
func parseArgs(reg Registry, args []string) (Options, Values, error) {
	var opts Options
	values := Values{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-h" {
			opts.Help = true
			continue
		}
		name, inline, hasInline := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if !strings.HasPrefix(arg, "--") || name == "" {
			return opts, nil, &usageError{"unknown argument: " + arg}
		}
		// value는 옵션의 값을 읽는다(--name=value 또는 다음 인자).
		value := func(allowEmpty bool) (string, error) {
			v := inline
			if !hasInline {
				if i+1 >= len(args) {
					return "", &usageError{"--" + name + " needs a value"}
				}
				i++
				v = args[i]
			}
			if v == "" && !allowEmpty {
				return "", &usageError{"--" + name + " needs a non-empty value"}
			}
			return v, nil
		}
		noValue := func() error {
			if hasInline {
				return &usageError{"--" + name + " does not take a value"}
			}
			return nil
		}
		var err error
		switch name {
		case "upgrade":
			err, opts.Upgrade = noValue(), true
		case "reinstall":
			err, opts.Reinstall = noValue(), true
		case "no-build":
			err, opts.NoBuild = noValue(), true
		case "dry-run":
			err, opts.DryRun = noValue(), true
		case "plain":
			err, opts.Plain = noValue(), true
		case "help":
			err, opts.Help = noValue(), true
		case "uninstall":
			err, opts.Action = noValue(), ActionUninstall
		case "service-install":
			err, opts.Action = noValue(), ActionServiceInstall
		case "lang":
			if opts.Lang, err = value(false); err == nil && opts.Lang != "en" && opts.Lang != "ko" {
				err = &usageError{"--lang must be en or ko"}
			}
		default:
			s, toggle := reg.flag(name)
			if s == nil {
				return opts, nil, &usageError{"unknown option: --" + name}
			}
			switch s.Flag {
			case FlagToggle:
				if err = noValue(); err == nil {
					values.Set(s.Name, toggle)
				}
			case FlagBool:
				if err = noValue(); err == nil {
					values.SetBool(s.Name, true)
				}
			default:
				var v string
				if v, err = value(s.Flag == FlagValueEmptyOK); err == nil {
					values.Set(s.Name, v)
				}
			}
		}
		if err != nil {
			return opts, nil, err
		}
	}
	switch {
	case opts.Upgrade && opts.Reinstall:
		return opts, nil, &usageError{"--upgrade and --reinstall cannot be used together"}
	case opts.Upgrade && opts.Action == ActionUninstall:
		return opts, nil, &usageError{"--upgrade cannot be used with --uninstall"}
	case opts.Reinstall && opts.Action == ActionUninstall:
		return opts, nil, &usageError{"--reinstall cannot be used with --uninstall"}
	}
	return opts, values, nil
}

// usage는 사용법이다. name은 안내할 명령(./install.sh)이다.
func usage(reg Registry, lang, name string) string {
	var b strings.Builder
	if lang == "ko" {
		fmt.Fprintf(&b, "사용법: %s [--upgrade | --reinstall] [--no-build | --uninstall | --service-install] [설정 옵션] [--lang en|ko]\n\n", name)
		b.WriteString(`  (옵션 없음)        설정을 묻고 빌드한 뒤 설치합니다. 이미 설치되어 있으면 설치된 설정으로 업그레이드합니다
                     (기존 설치에 없는 설정만 묻고, 요약 뒤에 업그레이드·설정 바꾸기·취소를 고릅니다)
                     설치 단계에서 필요하면 sudo를 사용합니다
  --reinstall        설치된 값을 기본값으로 모든 설정을 다시 묻고 다시 설치합니다
  --upgrade          설치된 설정으로 확인 없이 업그레이드합니다(스크립트용). 설치되어 있지 않으면 실패합니다
                     기존 설치에 없는 설정(새 버전에서 생긴 설정)만 묻습니다
  --no-build         빌드를 건너뛰고 기존 bin/onmaru를 설치합니다
  --uninstall        서비스, 실행 파일, 설정을 제거합니다
  --service-install  빌드 없이 설치 단계만 실행합니다(root 전용). 설정 옵션을 모두 주면 묻지 않습니다
  --dry-run          설치 단계를 실행하지 않고 쓸 파일의 차이와 실행할 명령만 보여 줍니다(sudo 없음.
                     --no-build가 없으면 빌드는 합니다)
  --plain            TUI 대신 줄 단위로 묻습니다
  --lang en|ko       안내 문구 언어(기본값은 로캘에 따름)
  -h, --help         이 도움말을 보여 줍니다

설정 옵션(주지 않은 항목만 묻습니다):
`)
		b.WriteString(reg.usageSettings(lang))
		b.WriteString("\n일반 사용자는 sudo 없이 실행하세요. 빌드는 그 사용자로 하고, 설치 단계만 sudo로 실행합니다.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Usage: %s [--upgrade | --reinstall] [--no-build | --uninstall | --service-install] [settings] [--lang en|ko]\n\n", name)
	b.WriteString(`  (no option)        ask for settings, build, then install. If onmaru is already installed, upgrade with the
                     installed settings (only missing settings are asked; after the summary choose
                     Upgrade, Change settings or Cancel). Uses sudo for the install step when needed
  --reinstall        ask every setting again, with the installed values as defaults, then reinstall
  --upgrade          upgrade with the installed settings without any confirmation (for scripts); fails if
                     nothing is installed. Only settings missing from the installation (new in this version) are asked
  --no-build         skip the build and install the existing bin/onmaru
  --uninstall        remove the service, binary and settings
  --service-install  run only the install step, without building (root only). No questions if all settings are given
  --dry-run          show the install step's file changes and commands instead of running them (no sudo;
                     the build still runs unless --no-build is given)
  --plain            ask line by line instead of the TUI
  --lang en|ko       language of the messages (default: from the locale)
  -h, --help         show this help

Settings (only the missing ones are asked):
`)
	b.WriteString(reg.usageSettings(lang))
	b.WriteString("\nNormal users should run it without sudo: the build runs as that user and only the install step uses sudo.\n")
	return b.String()
}
