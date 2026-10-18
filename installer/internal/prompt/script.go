package prompt

import (
	"fmt"
	"slices"
)

// Script는 테스트용 질문 방식이다. 질문 ID마다 미리 정한 답을 돌려주고, 물은 질문을 순서대로 남긴다.
// 답이 없는 질문은 오류다. 빈 답은 기본값을 고른 것이다. Confirm의 답은 "y" 또는 "n"이다.
type Script struct {
	Answers map[string]string
	// Asked는 물은 질문의 ID다.
	Asked []string
	// Notes는 보여 준 Note의 제목이다.
	Notes []string
}

func (s *Script) answer(id string) (string, error) {
	s.Asked = append(s.Asked, id)
	a, ok := s.Answers[id]
	if !ok {
		return "", fmt.Errorf("unexpected question %q", id)
	}
	return a, nil
}

// Input은 정한 답(빈 답이면 기본값)을 검사해 돌려준다. 검사를 통과하지 못하면 오류다.
func (s *Script) Input(q Input) (string, error) {
	a, err := s.answer(q.ID)
	if err != nil {
		return "", err
	}
	if a == "" {
		a = q.Default
	}
	if q.Validate != nil {
		if err := q.Validate(a); err != nil {
			return "", fmt.Errorf("question %q: answer %q rejected: %w", q.ID, a, err)
		}
	}
	return a, nil
}

// Select는 정한 답(선택지의 Value, 빈 답이면 기본값)을 돌려준다.
func (s *Script) Select(q Select) (string, error) {
	a, err := s.answer(q.ID)
	if err != nil {
		return "", err
	}
	if a == "" {
		a = q.Default
	}
	if !slices.ContainsFunc(q.Options, func(o Option) bool { return o.Value == a }) {
		return "", fmt.Errorf("question %q: %q is not an option", q.ID, a)
	}
	return a, nil
}

// Confirm은 정한 답("y", "n", 빈 답이면 기본값)을 돌려준다.
func (s *Script) Confirm(q Confirm) (bool, error) {
	a, err := s.answer(q.ID)
	if err != nil {
		return false, err
	}
	switch a {
	case "":
		return q.Default, nil
	case "y":
		return true, nil
	case "n":
		return false, nil
	}
	return false, fmt.Errorf("question %q: answer %q is not y or n", q.ID, a)
}

// Note는 제목만 남긴다.
func (s *Script) Note(title, _ string) error {
	s.Notes = append(s.Notes, title)
	return nil
}

// Fail은 어떤 질문이든 오류로 돌려주는 테스트용 질문 방식이다. 질문이 없어야 하는 흐름(설정을 모두 준
// --service-install 등)을 확인한다. Note는 질문이 아니므로 받는다.
type Fail struct{}

func (Fail) Input(q Input) (string, error)   { return "", fmt.Errorf("unexpected question %q", q.ID) }
func (Fail) Select(q Select) (string, error) { return "", fmt.Errorf("unexpected question %q", q.ID) }
func (Fail) Confirm(q Confirm) (bool, error) {
	return false, fmt.Errorf("unexpected question %q", q.ID)
}
func (Fail) Note(string, string) error { return nil }
