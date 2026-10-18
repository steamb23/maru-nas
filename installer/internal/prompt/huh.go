package prompt

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// Huh는 huh v2 TUI 질문이다. 표준 입력·출력이 터미널이고 TERM이 dumb가 아니며 --plain이 아닐 때 쓴다.
// 질문마다 폼 하나를 띄우고, 폼은 끝나면 화면에서 사라지므로 답한 내용을 한 줄씩 남긴다.
// 입력 중 검사는 Plain과 같은 Validate 함수다.
type Huh struct {
	out  io.Writer
	word words
	// label은 남기는 줄의 질문 부분 모양이다.
	label lipgloss.Style
}

// NewHuh는 TUI 질문을 만든다. lang은 en 또는 ko다.
func NewHuh(lang string) *Huh {
	return &Huh{
		out:   os.Stderr,
		word:  wordsFor(lang),
		label: lipgloss.NewStyle().Faint(true),
	}
}

func (h *Huh) run(f huh.Field) error {
	err := huh.NewForm(huh.NewGroup(f)).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrAborted
	}
	return err
}

// record는 답한 질문을 남긴다.
func (h *Huh) record(title, answer string) {
	fmt.Fprintf(h.out, "%s %s\n", h.label.Render(title+":"), answer)
}

// Input은 기본값을 미리 채운 입력란이다. 기본값이 "-"(끔)이면 채우지 않고 자리 표시로 보여 준다.
// 비우고 확인하면 기본값이다.
func (h *Huh) Input(q Input) (string, error) {
	v := q.Default
	in := huh.NewInput().Title(q.Title)
	if q.Default == "-" {
		v = ""
		in.Placeholder(q.Default)
	}
	in.Value(&v)
	if q.Help != "" {
		in.Description(q.Help)
	}
	resolve := func(s string) string {
		if s = strings.TrimSpace(s); s == "" {
			return q.Default
		}
		return s
	}
	if q.Validate != nil {
		in.Validate(func(s string) error { return q.Validate(resolve(s)) })
	}
	if err := h.run(in); err != nil {
		return "", err
	}
	v = resolve(v)
	h.record(q.Title, v)
	return v, nil
}

// Select는 선택 목록이다. 선택지 설명은 이름 뒤에 붙인다.
func (h *Huh) Select(q Select) (string, error) {
	v := q.Default
	opts := make([]huh.Option[string], len(q.Options))
	labels := map[string]string{}
	for i, o := range q.Options {
		label := o.Label
		if o.Help != "" {
			label += " - " + o.Help
		}
		opts[i] = huh.NewOption(label, o.Value)
		labels[o.Value] = o.Label
	}
	s := huh.NewSelect[string]().Title(q.Title).Options(opts...).Value(&v)
	if q.Help != "" {
		s.Description(q.Help)
	}
	if err := h.run(s); err != nil {
		return "", err
	}
	h.record(q.Title, labels[v])
	return v, nil
}

// Confirm은 예·아니요 단추다.
func (h *Huh) Confirm(q Confirm) (bool, error) {
	v := q.Default
	c := huh.NewConfirm().Title(q.Title).Affirmative(h.word.yes).Negative(h.word.no).Value(&v)
	if q.Help != "" {
		c.Description(q.Help)
	}
	if err := h.run(c); err != nil {
		return false, err
	}
	answer := h.word.no
	if v {
		answer = h.word.yes
	}
	h.record(q.Title, answer)
	return v, nil
}

// Note는 제목과 내용을 화면에 남긴다. 다음 질문(진행 확인 등)을 하는 동안 보이도록 폼으로 띄우지 않는다.
func (h *Huh) Note(title, body string) error {
	fmt.Fprintln(h.out)
	if title != "" {
		fmt.Fprintln(h.out, lipgloss.NewStyle().Bold(true).Render(title))
	}
	fmt.Fprintln(h.out, body)
	fmt.Fprintln(h.out)
	return nil
}
