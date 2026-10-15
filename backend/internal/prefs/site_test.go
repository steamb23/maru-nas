package prefs

import (
	"os"
	"path/filepath"
	"testing"

	"onmaru/backend/internal/api"
)

func TestSiteRoundTrip(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			site := s.(SiteStore)
			got, err := site.GetSite()
			if err != nil || got.Login != nil || got.Defaults != nil || got.Server != nil {
				t.Fatalf("empty: %+v, %v", got, err)
			}
			url := "https://nas.example.lan/"
			if err := site.UpdateSite(func(v *api.SiteSettings) {
				v.Login = &api.SiteLogin{Background: &api.Wallpaper{Kind: api.WallpaperPreset, Preset: "dusk"}}
				v.Server = &api.ServerOverrides{WorkbenchURL: &url}
			}); err != nil {
				t.Fatal(err)
			}
			// 한 부분을 바꿔도 다른 부분은 그대로다.
			if err := site.UpdateSite(func(v *api.SiteSettings) {
				v.Defaults = &api.SiteDefaults{Theme: api.ThemeDark}
			}); err != nil {
				t.Fatal(err)
			}
			got, _ = site.GetSite()
			if got.Login == nil || got.Login.Background.Preset != "dusk" || got.Login.EasterEgg ||
				got.Defaults == nil || got.Defaults.Theme != api.ThemeDark ||
				got.Server == nil || *got.Server.WorkbenchURL != url {
				t.Fatalf("after update: %+v", got)
			}
			// 받은 값을 바꿔도 저장값은 그대로다.
			got.Login.Background.Preset = "changed"
			*got.Server.WorkbenchURL = "changed"
			again, _ := site.GetSite()
			if again.Login.Background.Preset != "dusk" || *again.Server.WorkbenchURL != url {
				t.Errorf("stored value changed: %+v", again)
			}
		})
	}
}

func TestSiteFileLayout(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileStore(dir)
	if err := s.UpdateSite(func(v *api.SiteSettings) { v.Login = &api.SiteLogin{EasterEgg: true} }); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "site.json"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"version\": 1,\n  \"login\": {\n    \"easterEgg\": true\n  }\n}\n"; string(b) != want {
		t.Errorf("site.json = %q", b)
	}
	s2, _ := NewFileStore(dir)
	if got, err := s2.GetSite(); err != nil || got.Login == nil || !got.Login.EasterEgg {
		t.Errorf("reopen: %+v, %v", got, err)
	}
}

func TestSiteCorruptFile(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileStore(dir)
	path := filepath.Join(dir, "site.json")
	if err := os.WriteFile(path, []byte(`{"login": {"easterEgg": "yes"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetSite(); err != nil || got.Login != nil {
		t.Fatalf("corrupt: %+v, %v", got, err)
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Errorf("corrupt file not kept: %v", err)
	}
}
