package ghcli

import "strings"

// IsWSL reports whether we run under Windows Subsystem for Linux.
func IsWSL(getenv func(string) string, readFile func(string) ([]byte, error)) bool {
	if getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	b, err := readFile("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

// BrowserEnv is everything BrowserCommand needs to know about the machine.
type BrowserEnv struct {
	GOOS    string
	Browser string // $BROWSER
	WSL     bool
	Has     func(name string) bool // whether a program is on PATH
}

// BrowserCommand picks the program and arguments that open url: $BROWSER first, then wslview or
// cmd.exe under WSL, then the platform's own opener. The url is always a single argument.
func BrowserCommand(e BrowserEnv, url string) (name string, args []string) {
	if name, args, ok := fromBrowserVar(e, url); ok {
		return name, args
	}
	switch {
	case e.WSL && e.Has("wslview"):
		return "wslview", []string{url}
	case e.WSL:
		return "cmd.exe", []string{"/c", "start", "", url}
	case e.GOOS == "darwin":
		return "open", []string{url}
	case e.GOOS == "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	}
	return "xdg-open", []string{url}
}

// fromBrowserVar reads $BROWSER: alternatives separated by ':', the first one installed wins
// (else the first), words split on spaces, "%s" stands for the url.
func fromBrowserVar(e BrowserEnv, url string) (string, []string, bool) {
	var first []string
	for _, alt := range strings.Split(e.Browser, ":") {
		words := strings.Fields(alt)
		if len(words) == 0 {
			continue
		}
		if first == nil {
			first = words
		}
		if e.Has(words[0]) {
			first = words
			break
		}
	}
	if first == nil {
		return "", nil, false
	}
	var args []string
	placed := false
	for _, w := range first[1:] {
		if w == "%s" {
			w, placed = url, true
		}
		args = append(args, w)
	}
	if !placed {
		args = append(args, url)
	}
	return first[0], args, true
}
