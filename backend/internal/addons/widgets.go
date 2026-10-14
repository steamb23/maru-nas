package addons

import (
	"fmt"

	"onmaru/backend/internal/api"
)

const (
	// MaxWidgets는 한 사용자가 바탕화면에 둘 수 있는 위젯 수의 상한이다.
	MaxWidgets = 64
	// MaxWidgetCoord는 위젯 배치 좌표의 상한(칸)이다. 위젯의 오른쪽·아래쪽 끝도 이 값을 넘지 않는다.
	// 화면에 들어가지 않는 위젯은 프론트가 그리드 아래쪽으로 밀어 두므로 화면보다 넉넉하게 둔다.
	MaxWidgetCoord = 200
)

// ValidateWidgets는 저장할 위젯 배치를 검사한다. 각 위젯은 위젯 보기가 있는 등록된 애드온이고,
// 크기는 manifest 범위 안, 좌표는 0~MaxWidgetCoord, 애드온마다 하나, 서로 겹치지 않아야 한다.
// capability(역할·플러그인)는 보지 않는다. 역할이나 플러그인이 바뀌어도 배치를 잃지 않게 하고,
// 표시 여부는 프론트가 정한다.
func (r *Registry) ValidateWidgets(list []api.WidgetPlacement) error {
	if len(list) > MaxWidgets {
		return fmt.Errorf("too many widgets (max %d)", MaxWidgets)
	}
	seen := make(map[string]bool, len(list))
	for i, p := range list {
		a, ok := r.Get(p.Addon)
		if !ok || a.Manifest.Views.Widget == nil {
			return fmt.Errorf("widget %d: unknown widget addon %q", i, p.Addon)
		}
		if seen[p.Addon] {
			return fmt.Errorf("widget %d: duplicate widget addon %q", i, p.Addon)
		}
		seen[p.Addon] = true
		lo, hi := WidgetSizeRange(*a.Manifest.Views.Widget)
		if p.Cols < lo.Cols || p.Cols > hi.Cols || p.Rows < lo.Rows || p.Rows > hi.Rows {
			return fmt.Errorf("widget %d: size %dx%d out of range", i, p.Cols, p.Rows)
		}
		if p.Right < 0 || p.Top < 0 || p.Right+p.Cols > MaxWidgetCoord || p.Top+p.Rows > MaxWidgetCoord {
			return fmt.Errorf("widget %d: position out of range", i)
		}
		for j, q := range list[:i] {
			if widgetsOverlap(p, q) {
				return fmt.Errorf("widget %d overlaps widget %d", i, j)
			}
		}
	}
	return nil
}

// WidgetSizeRange는 위젯 보기의 최소·최대 크기다. 선언하지 않은 쪽은 기본 크기다.
func WidgetSizeRange(w api.WidgetView) (lo, hi api.GridSize) {
	lo, hi = w.DefaultSize, w.DefaultSize
	if w.MinSize != nil {
		lo = *w.MinSize
	}
	if w.MaxSize != nil {
		hi = *w.MaxSize
	}
	return lo, hi
}

// widgetsOverlap은 두 위젯이 같은 칸을 차지하는지다. 오른쪽 기준 좌표도 왼쪽 기준과 같은 방식으로 비교한다.
func widgetsOverlap(a, b api.WidgetPlacement) bool {
	return a.Right < b.Right+b.Cols && b.Right < a.Right+a.Cols &&
		a.Top < b.Top+b.Rows && b.Top < a.Top+a.Rows
}
