package icons

import (
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

func nerd(t *testing.T) {
	t.Helper()
	old := Current()
	Use(Nerd)
	t.Cleanup(func() { Use(old) })
}

func TestFileKinds(t *testing.T) {
	cases := map[string]string{
		"main.go": "go", "a/b/app.py": "python", "x.js": "javascript", "x.jsx": "react", "x.ts": "typescript",
		"x.tsx": "react", "x.d.ts": "typescript", "a.test.js": "test", "a.spec.ts": "test", "lib.rs": "rust",
		"A.java": "java", "a.c": "c", "a.h": "c", "a.cpp": "cpp", "a.cs": "csharp", "a.rb": "ruby", "a.php": "php",
		"a.swift": "swift", "a.kt": "kotlin", "a.sh": "shell", "a.lua": "lua", "README.md": "markdown",
		"a.json": "json", "a.yml": "config", "a.yaml": "config", "a.toml": "config", "a.html": "html",
		"a.css": "css", "a.scss": "sass", "a.sql": "sql", "Dockerfile": "docker", "Dockerfile.dev": "docker",
		"docker-compose.yml": "docker", "Makefile": "make", "go.mod": "go", "go.sum": "lock",
		"package.json": "npm", "package-lock.json": "lock", ".gitignore": "git", ".env": "env", "a.PNG": "image",
		"a.txt": "text", "LICENSE": "license", "noext": "file", ".hidden": "file", "weird.xyz": "file",
		"CMakeLists.txt": "make", ".env.local": "env", "x_test.go": "test", "a\\b\\c.GO": "go",
	}
	for p, want := range cases {
		if got := kindOf(p).label; got != want {
			t.Errorf("kindOf(%q) = %q, want %q", p, got, want)
		}
	}
}

func TestFileIconNerdAndPlain(t *testing.T) {
	Use(Plain)
	if g, c := FileIcon("main.go"); g != "" || c != "" {
		t.Errorf("plain file icon = %q %q", g, c)
	}
	if g, c := FolderIcon(true); g != "" || c != "" {
		t.Errorf("plain folder icon = %q %q", g, c)
	}
	nerd(t)
	g, c := FileIcon("main.go")
	if g == "" || c == "" {
		t.Fatalf("nerd go icon = %q %q", g, c)
	}
	if o, _ := FolderIcon(true); o == "" {
		t.Error("open folder has no glyph")
	}
	open, _ := FolderIcon(true)
	closed, _ := FolderIcon(false)
	if open == closed {
		t.Error("open and closed folder look the same")
	}
}

func TestFileGlyphsAreOneColumn(t *testing.T) {
	check := func(label, g string) {
		r, size := utf8.DecodeRuneInString(g)
		if size != len(g) || r < 0xE000 || r > 0xF8FF {
			t.Errorf("%s glyph %q is not one private-use rune", label, g)
		}
		if w := lipgloss.Width(g); w != 1 {
			t.Errorf("%s glyph %q has width %d", label, g, w)
		}
	}
	for ext, k := range fileKinds {
		check(ext, k.glyph)
		if k.color == "" {
			t.Errorf("%s has no colour", ext)
		}
	}
	for n, k := range fileNames {
		check(n, k.glyph)
	}
	check("generic", kGeneric.glyph)
	nerd(t)
	o, _ := FolderIcon(true)
	c, _ := FolderIcon(false)
	check("open folder", o)
	check("folder", c)
}
