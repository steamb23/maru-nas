package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"onmaru/backend/internal/api"
)

var sample = []api.WidgetPlacement{
	{Addon: "system-info", Right: 0, Top: 0, Cols: 3, Rows: 2},
	{Addon: "storage", Right: 3, Top: 0, Cols: 2, Rows: 2},
}

var sampleShortcuts = []api.Shortcut{{App: "system-info"}, {App: "files"}}

// stores는 같은 동작을 확인할 두 구현이다.
func stores(t *testing.T) map[string]Store {
	fs, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return map[string]Store{"memory": NewMemoryStore(), "file": fs}
}

func TestStoreRoundTrip(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			p, err := s.Get("alice")
			if err != nil || p.Widgets != nil {
				t.Fatalf("new user: %+v, %v (want no widgets)", p, err)
			}
			if err := s.SetWidgets("alice", sample); err != nil {
				t.Fatal(err)
			}
			p, err = s.Get("alice")
			if err != nil || p.Widgets == nil || !slices.Equal(*p.Widgets, sample) {
				t.Fatalf("after set: %+v, %v", p, err)
			}
			// 다른 사용자와 섞이지 않는다.
			if p, _ := s.Get("bob"); p.Widgets != nil {
				t.Errorf("bob sees %+v", *p.Widgets)
			}
			// 빈 목록은 "저장한 적 없음"과 구분한다.
			if err := s.SetWidgets("alice", nil); err != nil {
				t.Fatal(err)
			}
			p, _ = s.Get("alice")
			if p.Widgets == nil || len(*p.Widgets) != 0 {
				t.Errorf("after clear: %+v", p.Widgets)
			}
		})
	}
}

func TestStoreShortcutsRoundTrip(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			if p, err := s.Get("alice"); err != nil || p.Shortcuts != nil {
				t.Fatalf("new user: %+v, %v (want no shortcuts)", p, err)
			}
			if err := s.SetShortcuts("alice", sampleShortcuts); err != nil {
				t.Fatal(err)
			}
			p, err := s.Get("alice")
			if err != nil || p.Shortcuts == nil || !slices.Equal(*p.Shortcuts, sampleShortcuts) {
				t.Fatalf("after set: %+v, %v", p, err)
			}
			if p, _ := s.Get("bob"); p.Shortcuts != nil {
				t.Errorf("bob sees %+v", *p.Shortcuts)
			}
			// 빈 목록은 "저장한 적 없음"과 구분한다.
			if err := s.SetShortcuts("alice", nil); err != nil {
				t.Fatal(err)
			}
			p, _ = s.Get("alice")
			if p.Shortcuts == nil || len(*p.Shortcuts) != 0 {
				t.Errorf("after clear: %+v", p.Shortcuts)
			}
		})
	}
}

// 설정마다 따로 저장해도 다른 설정을 지우지 않는다.
func TestStoreFieldsIndependent(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			if err := s.SetWidgets("alice", sample); err != nil {
				t.Fatal(err)
			}
			if err := s.SetShortcuts("alice", sampleShortcuts); err != nil {
				t.Fatal(err)
			}
			p, _ := s.Get("alice")
			if p.Widgets == nil || !slices.Equal(*p.Widgets, sample) {
				t.Errorf("shortcuts dropped widgets: %+v", p.Widgets)
			}
			if err := s.SetWidgets("alice", nil); err != nil {
				t.Fatal(err)
			}
			p, _ = s.Get("alice")
			if p.Shortcuts == nil || !slices.Equal(*p.Shortcuts, sampleShortcuts) {
				t.Errorf("widgets dropped shortcuts: %+v", p.Shortcuts)
			}
		})
	}
}

func TestStoreDrawer(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			if p, _ := s.Get("alice"); p.Drawer != nil {
				t.Fatalf("new user drawer: %+v", *p.Drawer)
			}
			if err := s.SetShortcuts("alice", sampleShortcuts); err != nil {
				t.Fatal(err)
			}
			if err := s.SetDrawer("alice", api.DrawerPrefs{Sort: api.DrawerSortName}); err != nil {
				t.Fatal(err)
			}
			p, err := s.Get("alice")
			if err != nil || p.Drawer == nil || p.Drawer.Sort != api.DrawerSortName {
				t.Fatalf("after set: %+v, %v", p.Drawer, err)
			}
			if p.Shortcuts == nil || !slices.Equal(*p.Shortcuts, sampleShortcuts) {
				t.Errorf("drawer save dropped shortcuts: %+v", p.Shortcuts)
			}
			// 돌려준 값을 바꿔도 저장값은 그대로다.
			p.Drawer.Sort = "changed"
			if p, _ := s.Get("alice"); p.Drawer.Sort != api.DrawerSortName {
				t.Errorf("stored drawer changed: %+v", *p.Drawer)
			}
			if p, _ := s.Get("bob"); p.Drawer != nil {
				t.Errorf("bob sees %+v", *p.Drawer)
			}
		})
	}
}

func TestStoreSystemInfo(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			if p, _ := s.Get("alice"); p.SystemInfo != nil {
				t.Fatalf("new user system info: %+v", *p.SystemInfo)
			}
			if err := s.SetDrawer("alice", api.DrawerPrefs{Sort: api.DrawerSortName}); err != nil {
				t.Fatal(err)
			}
			want := api.SystemInfoPrefs{CPUScale: api.CPUScaleCore, CPUGraph: api.CPUGraphCores}
			if err := s.SetSystemInfo("alice", want); err != nil {
				t.Fatal(err)
			}
			p, err := s.Get("alice")
			if err != nil || p.SystemInfo == nil || *p.SystemInfo != want {
				t.Fatalf("after set: %+v, %v", p.SystemInfo, err)
			}
			if p.Drawer == nil || p.Drawer.Sort != api.DrawerSortName {
				t.Errorf("system info save dropped drawer: %+v", p.Drawer)
			}
			// 돌려준 값을 바꿔도 저장값은 그대로다.
			p.SystemInfo.CPUScale = "changed"
			if p, _ := s.Get("alice"); *p.SystemInfo != want {
				t.Errorf("stored system info changed: %+v", *p.SystemInfo)
			}
			if p, _ := s.Get("bob"); p.SystemInfo != nil {
				t.Errorf("bob sees %+v", *p.SystemInfo)
			}
		})
	}
}

func TestStoreReturnsCopies(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			in := slices.Clone(sample)
			if err := s.SetWidgets("alice", in); err != nil {
				t.Fatal(err)
			}
			sc := slices.Clone(sampleShortcuts)
			if err := s.SetShortcuts("alice", sc); err != nil {
				t.Fatal(err)
			}
			in[0].Top = 9
			sc[0].App = "changed"
			p, _ := s.Get("alice")
			(*p.Widgets)[1].Top = 9
			(*p.Shortcuts)[1].App = "changed"
			p, _ = s.Get("alice")
			if !slices.Equal(*p.Widgets, sample) {
				t.Errorf("stored value changed: %+v", *p.Widgets)
			}
			if !slices.Equal(*p.Shortcuts, sampleShortcuts) {
				t.Errorf("stored shortcuts changed: %+v", *p.Shortcuts)
			}
		})
	}
}

func TestFileStoreLayout(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetWidgets("alice", sample); err != nil {
		t.Fatal(err)
	}
	if err := s.SetShortcuts("alice", sampleShortcuts); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "users", "alice.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version   int               `json:"version"`
		Widgets   []json.RawMessage `json:"widgets"`
		Shortcuts []api.Shortcut    `json:"shortcuts"`
	}
	if err := json.Unmarshal(b, &doc); err != nil || doc.Version != fileVersion || len(doc.Widgets) != 2 ||
		!slices.Equal(doc.Shortcuts, sampleShortcuts) {
		t.Errorf("file = %s (%v)", b, err)
	}
	// 임시 파일이 남지 않는다.
	entries, _ := os.ReadDir(filepath.Join(dir, "users"))
	if len(entries) != 1 {
		t.Errorf("users dir has %d entries", len(entries))
	}
	// 다시 열어도 그대로 읽힌다.
	s2, _ := NewFileStore(dir)
	if p, err := s2.Get("alice"); err != nil || !slices.Equal(*p.Widgets, sample) {
		t.Errorf("reopen: %+v, %v", p, err)
	}
}

func TestFileName(t *testing.T) {
	cases := map[string]string{
		"alice":       "alice",
		"Bob_2-x":     "Bob_2-x",
		"john.doe":    "john%2Edoe",
		"..":          "%2E%2E",
		"a/b":         "a%2Fb",
		"100%":        "100%25",
		"DOMAIN\\usr": "DOMAIN%5Cusr",
		"사용자":         "%EC%82%AC%EC%9A%A9%EC%9E%90",
	}
	for in, want := range cases {
		if got := fileName(in); got != want {
			t.Errorf("fileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFileStoreStaysInDir(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileStore(dir)
	for _, name := range []string{"..", "../x", "a/../../b"} {
		if err := s.SetWidgets(name, sample); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "users" {
		t.Errorf("state dir entries: %v", entries)
	}
	if _, err := s.Get(""); err == nil {
		t.Error("empty username accepted")
	}
}

func TestFileStoreCorruptFile(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileStore(dir)
	path := filepath.Join(dir, "users", "alice.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := s.Get("alice")
	if err != nil || p.Widgets != nil {
		t.Fatalf("corrupt: %+v, %v", p, err)
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Errorf("corrupt file not kept: %v", err)
	}
	if err := s.SetWidgets("alice", sample); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Get("alice"); !slices.Equal(*p.Widgets, sample) {
		t.Errorf("after rewrite: %+v", p)
	}
}

func TestAppearanceRoundTrip(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			if err := s.SetDrawer("alice", api.DrawerPrefs{Sort: api.DrawerSortName}); err != nil {
				t.Fatal(err)
			}
			off := false
			if err := s.UpdateAppearance("alice", func(a *api.AppearancePrefs) {
				*a = api.AppearancePrefs{
					Theme:         api.ThemeDark,
					Wallpaper:     &api.Wallpaper{Kind: api.WallpaperImage},
					Animations:    &off,
					TaskbarAlign:  api.TaskbarAlignLeft,
					WidgetsLocked: true,
				}
			}); err != nil {
				t.Fatal(err)
			}
			// 조건부로 고칠 때는 지금 값을 받는다.
			if err := s.UpdateAppearance("alice", func(a *api.AppearancePrefs) {
				if a.Wallpaper != nil && a.Wallpaper.Kind == api.WallpaperImage {
					a.Wallpaper = nil
				}
			}); err != nil {
				t.Fatal(err)
			}
			p, err := s.Get("alice")
			if err != nil || p.Appearance == nil || p.Appearance.Theme != api.ThemeDark || p.Appearance.Wallpaper != nil ||
				p.Appearance.Animations == nil || *p.Appearance.Animations || p.Appearance.TaskbarAlign != api.TaskbarAlignLeft ||
				!p.Appearance.WidgetsLocked {
				t.Fatalf("appearance %+v, %v", p.Appearance, err)
			}
			// 다른 설정은 그대로다.
			if p.Drawer == nil || p.Drawer.Sort != api.DrawerSortName {
				t.Errorf("drawer %+v", p.Drawer)
			}
			// 받은 값을 바꿔도 저장값은 그대로다.
			*p.Appearance.Animations = true
			if again, _ := s.Get("alice"); *again.Appearance.Animations {
				t.Error("stored value changed")
			}
		})
	}
}

func TestFileStoreDoesNotStoreImageInfo(t *testing.T) {
	// 이미지 정보(wallpaperImage, avatarImage)는 응답할 때만 붙이는 값이라 파일에 저장하지 않는다.
	dir := t.TempDir()
	s, _ := NewFileStore(dir)
	path := filepath.Join(dir, "users", "alice.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"wallpaperImage":{"rev":"a"},"avatarImage":{"rev":"b"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if p, err := s.Get("alice"); err != nil || p.WallpaperImage != nil || p.AvatarImage != nil {
		t.Fatalf("read: %+v, %v", p, err)
	}
	if err := s.SetDrawer("alice", api.DrawerPrefs{Sort: api.DrawerSortName}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "Image") {
		t.Errorf("file = %s", b)
	}
}

func TestStoreFiles(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			if p, _ := s.Get("alice"); p.Files != nil {
				t.Fatalf("new user files prefs: %+v", *p.Files)
			}
			if err := s.SetDrawer("alice", api.DrawerPrefs{Sort: api.DrawerSortName}); err != nil {
				t.Fatal(err)
			}
			want := api.FilesPrefs{View: api.FilesViewGrid}
			if err := s.SetFiles("alice", want); err != nil {
				t.Fatal(err)
			}
			p, err := s.Get("alice")
			if err != nil || p.Files == nil || *p.Files != want {
				t.Fatalf("after set: %+v, %v", p.Files, err)
			}
			if p.Drawer == nil || p.Drawer.Sort != api.DrawerSortName {
				t.Errorf("files prefs save dropped drawer: %+v", p.Drawer)
			}
			// 돌려준 값을 바꿔도 저장값은 그대로다.
			p.Files.View = "changed"
			if p, _ := s.Get("alice"); *p.Files != want {
				t.Errorf("stored files prefs changed: %+v", *p.Files)
			}
			if p, _ := s.Get("bob"); p.Files != nil {
				t.Errorf("bob sees %+v", *p.Files)
			}
		})
	}
}
