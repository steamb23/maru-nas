package addons

import (
	"testing"

	"maru/backend/internal/api"
)

func widgetRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry(
		addon("cpu", func(a *Addon) {
			a.Manifest.Views.Widget = &api.WidgetView{
				DefaultSize: api.GridSize{Cols: 3, Rows: 2},
				MinSize:     &api.GridSize{Cols: 2, Rows: 2},
				MaxSize:     &api.GridSize{Cols: 4, Rows: 3},
			}
		}),
		// 크기를 바꿀 수 없는 위젯
		addon("clock", func(a *Addon) {
			a.Manifest.Views.Window = nil
			a.Manifest.Views.Widget = &api.WidgetView{DefaultSize: api.GridSize{Cols: 2, Rows: 1}}
		}),
		addon("window-only", nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestValidateWidgetsAccepts(t *testing.T) {
	r := widgetRegistry(t)
	cases := map[string][]api.WidgetPlacement{
		"empty": {},
		"side by side": {
			{Addon: "cpu", Right: 0, Top: 0, Cols: 3, Rows: 2},
			{Addon: "clock", Right: 3, Top: 0, Cols: 2, Rows: 1},
		},
		"stacked": {
			{Addon: "cpu", Right: 0, Top: 0, Cols: 4, Rows: 3},
			{Addon: "clock", Right: 0, Top: 3, Cols: 2, Rows: 1},
		},
		"min size": {{Addon: "cpu", Cols: 2, Rows: 2}},
		"far edge": {{Addon: "cpu", Right: MaxWidgetCoord - 3, Top: MaxWidgetCoord - 2, Cols: 3, Rows: 2}},
	}
	for name, list := range cases {
		if err := r.ValidateWidgets(list); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestValidateWidgetsRejects(t *testing.T) {
	r := widgetRegistry(t)
	cpu := api.WidgetPlacement{Addon: "cpu", Cols: 3, Rows: 2}
	with := func(edit func(*api.WidgetPlacement)) []api.WidgetPlacement {
		p := cpu
		edit(&p)
		return []api.WidgetPlacement{p}
	}
	tooMany := make([]api.WidgetPlacement, MaxWidgets+1)
	cases := map[string][]api.WidgetPlacement{
		"unknown addon":   with(func(p *api.WidgetPlacement) { p.Addon = "nope" }),
		"no widget view":  with(func(p *api.WidgetPlacement) { p.Addon = "window-only" }),
		"duplicate":       {cpu, {Addon: "cpu", Right: 5, Cols: 3, Rows: 2}},
		"below min":       with(func(p *api.WidgetPlacement) { p.Cols = 1 }),
		"above max":       with(func(p *api.WidgetPlacement) { p.Rows = 4 }),
		"fixed resized":   {{Addon: "clock", Cols: 3, Rows: 1}},
		"negative right":  with(func(p *api.WidgetPlacement) { p.Right = -1 }),
		"negative top":    with(func(p *api.WidgetPlacement) { p.Top = -1 }),
		"right overflow":  with(func(p *api.WidgetPlacement) { p.Right = MaxWidgetCoord - 2 }),
		"bottom overflow": with(func(p *api.WidgetPlacement) { p.Top = MaxWidgetCoord - 1 }),
		"overlap":         {cpu, {Addon: "clock", Right: 2, Top: 1, Cols: 2, Rows: 1}},
		"too many":        tooMany,
	}
	for name, list := range cases {
		if err := r.ValidateWidgets(list); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
