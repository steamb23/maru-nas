// Package prompt는 설치기가 사용자에게 묻는 방법이다. 줄 단위(Plain), huh TUI(Huh), 테스트용 구현(Script, Fail)이
// 같은 Prompter 인터페이스를 따른다. 질문을 할 수 있는지(터미널인지)는 부르는 쪽이 정한다.
package prompt

import "errors"

var (
	// ErrAborted는 사용자가 질문을 중단했다는 뜻이다(Ctrl+C).
	ErrAborted = errors.New("aborted")
	// ErrNoInput은 답을 읽기 전에 입력이 끝났다는 뜻이다(EOF).
	ErrNoInput = errors.New("no input (stdin closed)")
)

// Input은 한 줄 입력 질문이다.
type Input struct {
	// ID는 질문 식별자다. 테스트용 Script가 답을 고르는 데 쓴다.
	ID    string
	Title string
	// Help는 질문 앞에 보여 줄 설명이다(여러 줄 가능). 비어도 된다.
	Help string
	// Default는 빈 입력일 때 쓰는 값이다.
	Default string
	// Validate는 기본값을 적용한 입력을 검사한다. 오류면 다시 묻는다. nil이면 검사하지 않는다.
	Validate func(string) error
}

// Option은 Select의 선택지 하나다.
type Option struct {
	// Value는 돌려줄 값이다.
	Value string
	Label string
	// Help는 선택지 아래에 보여 줄 설명이다. 비어도 된다.
	Help string
}

// Select는 선택지 중 하나를 고르는 질문이다.
type Select struct {
	ID      string
	Title   string
	Help    string
	Options []Option
	// Default는 기본으로 고른 선택지의 Value다.
	Default string
}

// Confirm은 예·아니요 질문이다.
type Confirm struct {
	ID      string
	Title   string
	Help    string
	Default bool
}

// Prompter는 질문 방식이다. 질문을 중단하면 ErrAborted, 입력이 끝나면 ErrNoInput을 돌려준다.
type Prompter interface {
	Input(q Input) (string, error)
	Select(q Select) (string, error)
	Confirm(q Confirm) (bool, error)
	// Note는 질문 없이 내용을 보여 준다(요약 등).
	Note(title, body string) error
}

// words는 질문 방식이 직접 쓰는 짧은 문구다(언어별).
type words struct {
	choose, invalidChoice, yes, no, yesNoHint, noYesHint, answerYesNo string
}

func wordsFor(lang string) words {
	if lang == "ko" {
		return words{
			choose: "선택", invalidChoice: "잘못된 선택입니다", yes: "예", no: "아니요",
			yesNoHint: "Y/n", noYesHint: "y/N", answerYesNo: "y 또는 n으로 답하세요",
		}
	}
	return words{
		choose: "Choose", invalidChoice: "invalid choice", yes: "Yes", no: "No",
		yesNoHint: "Y/n", noYesHint: "y/N", answerYesNo: "answer y or n",
	}
}
