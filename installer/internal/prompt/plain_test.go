package prompt

import (
	"errors"
	"strings"
	"testing"
)

func TestPlainInputDefaultAndRetry(t *testing.T) {
	var out strings.Builder
	p := NewPlain(strings.NewReader("bad\n\n"), &out, "en")
	v, err := p.Input(Input{Title: "Name", Default: "maru", Validate: func(s string) error {
		if s == "bad" {
			return errors.New("not allowed")
		}
		return nil
	}})
	if err != nil || v != "maru" {
		t.Fatalf("got %q %v", v, err)
	}
	if !strings.Contains(out.String(), "Name [maru]: ") || !strings.Contains(out.String(), "not allowed") {
		t.Errorf("output %q", out.String())
	}
}

func TestPlainInputTrimsAndEOF(t *testing.T) {
	var out strings.Builder
	p := NewPlain(strings.NewReader("  value  "), &out, "en")
	if v, err := p.Input(Input{Title: "X"}); err != nil || v != "value" {
		t.Fatalf("got %q %v", v, err)
	}
	if _, err := p.Input(Input{Title: "X"}); !errors.Is(err, ErrNoInput) {
		t.Fatalf("err %v, want ErrNoInput", err)
	}
}

func TestPlainSelect(t *testing.T) {
	var out strings.Builder
	p := NewPlain(strings.NewReader("4\nx\n\n3\n"), &out, "ko")
	q := Select{Title: "Mode", Default: "b", Options: []Option{{Value: "a", Label: "A"}, {Value: "b", Label: "B", Help: "bee"}, {Value: "c", Label: "C"}}}
	if v, err := p.Select(q); err != nil || v != "b" {
		t.Fatalf("got %q %v", v, err)
	}
	if v, err := p.Select(q); err != nil || v != "c" {
		t.Fatalf("got %q %v", v, err)
	}
	s := out.String()
	for _, want := range []string{"  2) B\n     bee\n", "선택 1-3 [2]: ", "잘못된 선택입니다: 4", "잘못된 선택입니다: x"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
}

func TestPlainConfirm(t *testing.T) {
	var out strings.Builder
	p := NewPlain(strings.NewReader("\nYES\nmaybe\nn\n"), &out, "en")
	for _, want := range []bool{true, true, false} {
		v, err := p.Confirm(Confirm{Title: "Go?", Default: true})
		if err != nil || v != want {
			t.Fatalf("got %v %v, want %v", v, err, want)
		}
	}
	if !strings.Contains(out.String(), "Go? [Y/n]: ") || !strings.Contains(out.String(), "answer y or n") {
		t.Errorf("output %q", out.String())
	}
	if _, err := p.Confirm(Confirm{Title: "Go?"}); !errors.Is(err, ErrNoInput) {
		t.Fatalf("err %v", err)
	}
}

func TestScript(t *testing.T) {
	s := &Script{Answers: map[string]string{"a": "", "b": "n", "c": "x"}}
	if v, err := s.Input(Input{ID: "a", Default: "d"}); err != nil || v != "d" {
		t.Fatalf("got %q %v", v, err)
	}
	if v, err := s.Confirm(Confirm{ID: "b", Default: true}); err != nil || v {
		t.Fatalf("got %v %v", v, err)
	}
	if _, err := s.Select(Select{ID: "c", Options: []Option{{Value: "y"}}}); err == nil {
		t.Fatal("non-option accepted")
	}
	if _, err := s.Input(Input{ID: "missing"}); err == nil {
		t.Fatal("unexpected question accepted")
	}
	if strings.Join(s.Asked, ",") != "a,b,c,missing" {
		t.Errorf("asked %v", s.Asked)
	}
	if _, err := (Fail{}).Confirm(Confirm{ID: "x"}); err == nil {
		t.Fatal("Fail answered")
	}
}
