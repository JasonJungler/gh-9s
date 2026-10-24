package ghcli

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestIsWSL(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	file := func(s string, err error) func(string) ([]byte, error) {
		return func(string) ([]byte, error) { return []byte(s), err }
	}
	cases := []struct {
		name string
		env  map[string]string
		proc func(string) ([]byte, error)
		want bool
	}{
		{"distro env", map[string]string{"WSL_DISTRO_NAME": "Ubuntu"}, file("", errors.New("x")), true},
		{"wsl2 proc", nil, file("Linux version 5.15.90.1-microsoft-standard-WSL2", nil), true},
		{"wsl1 proc", nil, file("Linux version 4.4.0-19041-Microsoft", nil), true},
		{"plain linux", nil, file("Linux version 6.1.0-debian", nil), false},
		{"unreadable", nil, file("", errors.New("no such file")), false},
	}
	for _, c := range cases {
		if got := IsWSL(env(c.env), c.proc); got != c.want {
			t.Errorf("%s: IsWSL = %v; want %v", c.name, got, c.want)
		}
	}
}

func TestBrowserCommand(t *testing.T) {
	const url = "https://github.com/a/b/pull/1?x=1&y=2"
	has := func(names ...string) func(string) bool {
		return func(n string) bool {
			for _, x := range names {
				if x == n {
					return true
				}
			}
			return false
		}
	}
	cases := []struct {
		name     string
		env      BrowserEnv
		wantName string
		wantArgs []string
	}{
		{"darwin", BrowserEnv{GOOS: "darwin", Has: has()}, "open", []string{url}},
		{"windows", BrowserEnv{GOOS: "windows", Has: has()}, "rundll32", []string{"url.dll,FileProtocolHandler", url}},
		{"linux", BrowserEnv{GOOS: "linux", Has: has()}, "xdg-open", []string{url}},
		{"wsl with wslview", BrowserEnv{GOOS: "linux", WSL: true, Has: has("wslview")}, "wslview", []string{url}},
		{"wsl without wslview", BrowserEnv{GOOS: "linux", WSL: true, Has: has()}, "cmd.exe", []string{"/c", "start", "", url}},
		{"browser with args", BrowserEnv{GOOS: "linux", WSL: true, Browser: "firefox --new-tab", Has: has("wslview")}, "firefox", []string{"--new-tab", url}},
		{"alternatives, second present", BrowserEnv{GOOS: "linux", Browser: "a:b", Has: has("b")}, "b", []string{url}},
		{"alternatives, none present", BrowserEnv{GOOS: "linux", Browser: "a:b", Has: has()}, "a", []string{url}},
		{"placeholder", BrowserEnv{GOOS: "linux", Browser: "mybrowser %s --private", Has: has()}, "mybrowser", []string{url, "--private"}},
		{"blank is unset", BrowserEnv{GOOS: "linux", Browser: "   ", Has: has()}, "xdg-open", []string{url}},
	}
	for _, c := range cases {
		name, args := BrowserCommand(c.env, url)
		if name != c.wantName || !reflect.DeepEqual(args, c.wantArgs) {
			t.Errorf("%s: got %s %q; want %s %q", c.name, name, args, c.wantName, c.wantArgs)
		}
	}
}

func recordStart(t *testing.T, err error) *[]string {
	t.Helper()
	var got []string
	old := startCommand
	startCommand = func(name string, args ...string) error {
		got = append([]string{name}, args...)
		return err
	}
	t.Cleanup(func() { startCommand = old })
	return &got
}

func TestOpenURLStartsTheChosenCommand(t *testing.T) {
	t.Setenv("BROWSER", "fakebrowser")
	got := recordStart(t, nil)
	if err := OpenURL("https://x.test/?a=1&b=2"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"fakebrowser", "https://x.test/?a=1&b=2"}; !reflect.DeepEqual(*got, want) {
		t.Errorf("started %q; want %q", *got, want)
	}
}

func TestOpenURLReturnsTheStartError(t *testing.T) {
	t.Setenv("BROWSER", "fakebrowser")
	recordStart(t, errors.New("executable file not found"))
	err := OpenURL("https://x.test")
	if err == nil || !strings.Contains(err.Error(), "fakebrowser") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v; want it to name the command and the cause", err)
	}
}
