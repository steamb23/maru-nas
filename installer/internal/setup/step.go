package setup

// Phase는 단계를 실행하는 권한이다.
type Phase int

const (
	// PhaseUser는 실행한 계정으로 하는 단계다(빌드). root가 아니어도 된다.
	PhaseUser Phase = iota
	// PhaseRoot는 root로 하는 단계다(2단계 설치, 제거).
	PhaseRoot
)

// Step은 설치·제거 단계 하나다. 새 단계는 step_<이름>.go에 구현하고 steps.go의 목록에 한 줄 더한다.
type Step struct {
	// Name은 단계 이름이다(테스트·디버깅용).
	Name  string
	Phase Phase
	// When은 이 단계를 실행할 조건이다. nil이면 항상이다.
	When func(c *Context) bool
	Run  func(c *Context) error
}

// runSteps는 phase의 단계를 순서대로 실행한다. 처음 실패한 단계에서 멈춘다.
func runSteps(c *Context, steps []Step, phase Phase) error {
	for _, s := range steps {
		if s.Phase != phase || (s.When != nil && !s.When(c)) {
			continue
		}
		if err := s.Run(c); err != nil {
			return err
		}
	}
	return nil
}

// helperWanted는 도우미를 설치하기로 했는지다.
func helperWanted(c *Context) bool { return c.Values.Get("helper") == "yes" }
