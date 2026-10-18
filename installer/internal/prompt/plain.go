package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Plain은 줄 단위 질문이다. 비TTY, TERM=dumb, --plain일 때 쓴다. 잘못된 입력은 이유를 보여 주고 다시 묻는다.
type Plain struct {
	in   *bufio.Reader
	out  io.Writer
	word words
}

// NewPlain은 in에서 답을 읽고 out에 질문을 쓰는 Plain을 만든다. lang은 en 또는 ko다.
func NewPlain(in io.Reader, out io.Writer, lang string) *Plain {
	return &Plain{in: bufio.NewReader(in), out: out, word: wordsFor(lang)}
}

// readLine은 한 줄을 읽는다. bash의 read처럼 앞뒤 공백을 지운다.
func (p *Plain) readLine() (string, error) {
	line, err := p.in.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			if line != "" {
				return strings.TrimSpace(line), nil
			}
			fmt.Fprintln(p.out)
			return "", ErrNoInput
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (p *Plain) help(s string) {
	if s != "" {
		fmt.Fprintln(p.out, s)
	}
}

func (p *Plain) invalid(msg string) {
	fmt.Fprintf(p.out, "  -> %s\n", msg)
}

// Input은 "제목 [기본값]: "으로 묻는다. 빈 입력은 기본값이다.
func (p *Plain) Input(q Input) (string, error) {
	p.help(q.Help)
	for {
		if q.Default != "" {
			fmt.Fprintf(p.out, "%s [%s]: ", q.Title, q.Default)
		} else {
			fmt.Fprintf(p.out, "%s: ", q.Title)
		}
		v, err := p.readLine()
		if err != nil {
			return "", err
		}
		if v == "" {
			v = q.Default
		}
		if q.Validate != nil {
			if err := q.Validate(v); err != nil {
				p.invalid(err.Error())
				continue
			}
		}
		return v, nil
	}
}

// Select는 번호 목록을 보여 주고 번호로 고르게 한다.
func (p *Plain) Select(q Select) (string, error) {
	fmt.Fprintln(p.out)
	fmt.Fprintln(p.out, q.Title)
	p.help(q.Help)
	def := ""
	for i, o := range q.Options {
		fmt.Fprintf(p.out, "  %d) %s\n", i+1, o.Label)
		if o.Help != "" {
			fmt.Fprintf(p.out, "     %s\n", o.Help)
		}
		if o.Value == q.Default {
			def = strconv.Itoa(i + 1)
		}
	}
	for {
		prompt := fmt.Sprintf("%s 1-%d", p.word.choose, len(q.Options))
		if def != "" {
			fmt.Fprintf(p.out, "%s [%s]: ", prompt, def)
		} else {
			fmt.Fprintf(p.out, "%s: ", prompt)
		}
		v, err := p.readLine()
		if err != nil {
			return "", err
		}
		if v == "" {
			v = def
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > len(q.Options) {
			p.invalid(fmt.Sprintf("%s: %s", p.word.invalidChoice, v))
			continue
		}
		return q.Options[n-1].Value, nil
	}
}

// Confirm은 "제목 [Y/n]: "으로 묻는다. y·yes·n·no(대소문자 무관)만 받고, 빈 입력은 기본값이다.
func (p *Plain) Confirm(q Confirm) (bool, error) {
	p.help(q.Help)
	hint := p.word.noYesHint
	if q.Default {
		hint = p.word.yesNoHint
	}
	for {
		fmt.Fprintf(p.out, "%s [%s]: ", q.Title, hint)
		v, err := p.readLine()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(v) {
		case "":
			return q.Default, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		p.invalid(p.word.answerYesNo)
	}
}

// Note는 제목과 내용을 그대로 쓴다.
func (p *Plain) Note(title, body string) error {
	fmt.Fprintln(p.out)
	if title != "" {
		fmt.Fprintln(p.out, title)
	}
	fmt.Fprintln(p.out, body)
	return nil
}
