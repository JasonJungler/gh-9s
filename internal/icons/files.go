package icons

import (
	"path"
	"strings"
)

// fileKind is how a kind of file is drawn in the Nerd set.
type fileKind struct {
	label string
	glyph string
	color string
}

// The glyphs are Seti-UI (nf-seti-*), devicons (nf-dev-*) and Font Awesome (nf-fa-*) code points from
// the Nerd Fonts v3 cheat sheet, all in the BMP private use area so each is one column wide. The
// colours are the usual ones of each language.
var (
	kGeneric  = fileKind{"file", "", "245"}           // nf-fa-file_o
	kText     = fileKind{"text", "", "245"}           // nf-fa-file_text_o
	kGo       = fileKind{"go", "", "#00ADD8"}         // nf-dev-go
	kPython   = fileKind{"python", "", "#FFBC03"}     // nf-dev-python
	kJS       = fileKind{"javascript", "", "#F1E05A"} // nf-dev-javascript_badge
	kTS       = fileKind{"typescript", "", "#3178C6"} // nf-seti-typescript
	kReact    = fileKind{"react", "", "#61DAFB"}      // nf-dev-react
	kRust     = fileKind{"rust", "", "#DEA584"}       // nf-dev-rust
	kJava     = fileKind{"java", "", "#CC3E44"}       // nf-dev-java
	kC        = fileKind{"c", "", "#599EFF"}          // nf-custom-c
	kCpp      = fileKind{"cpp", "", "#F34B7D"}        // nf-custom-cpp
	kCSharp   = fileKind{"csharp", "", "#68217A"}     // nf-seti-c_sharp
	kRuby     = fileKind{"ruby", "", "#CC342D"}       // nf-dev-ruby
	kPHP      = fileKind{"php", "", "#A074C4"}        // nf-dev-php
	kSwift    = fileKind{"swift", "", "#E37933"}      // nf-dev-swift
	kKotlin   = fileKind{"kotlin", "", "#7F52FF"}     // nf-seti-kotlin
	kShell    = fileKind{"shell", "", "#89E051"}      // nf-dev-terminal
	kLua      = fileKind{"lua", "", "#51A0CF"}        // nf-seti-lua
	kMarkdown = fileKind{"markdown", "", "#519ABA"}   // nf-dev-markdown
	kJSON     = fileKind{"json", "", "#CBCB41"}       // nf-seti-json
	kConfig   = fileKind{"config", "", "#6D8086"}     // nf-seti-config
	kHTML     = fileKind{"html", "", "#E44D26"}       // nf-dev-html5
	kCSS      = fileKind{"css", "", "#42A5F5"}        // nf-dev-css3
	kSass     = fileKind{"sass", "", "#F55385"}       // nf-dev-sass
	kSQL      = fileKind{"sql", "", "#DAD8D8"}        // nf-dev-database
	kDocker   = fileKind{"docker", "", "#458EE6"}     // nf-dev-docker
	kMake     = fileKind{"make", "", "#6D8086"}       // nf-fa-cogs
	kGit      = fileKind{"git", "", "#F14C28"}        // nf-dev-git
	kNpm      = fileKind{"npm", "", "#CB3837"}        // nf-dev-npm
	kLock     = fileKind{"lock", "", "#BBBBBB"}       // nf-fa-lock
	kImage    = fileKind{"image", "", "#A074C4"}      // nf-fa-file_image_o
	kTest     = fileKind{"test", "", "#4EC9B0"}       // nf-fa-flask
	kLicense  = fileKind{"license", "", "#D0BF41"}    // nf-fa-certificate
	kEnv      = fileKind{"env", "", "#FAF743"}        // nf-fa-cog
)

// fileKinds maps a lower-case extension (with the dot) to how such files are drawn.
var fileKinds = map[string]fileKind{
	".go": kGo, ".py": kPython, ".pyi": kPython, ".js": kJS, ".mjs": kJS, ".cjs": kJS, ".jsx": kReact,
	".ts": kTS, ".mts": kTS, ".cts": kTS, ".tsx": kReact, ".rs": kRust, ".java": kJava, ".c": kC,
	".h": kC, ".cpp": kCpp, ".cc": kCpp, ".cxx": kCpp, ".hpp": kCpp, ".cs": kCSharp, ".rb": kRuby,
	".php": kPHP, ".swift": kSwift, ".kt": kKotlin, ".kts": kKotlin, ".sh": kShell, ".bash": kShell,
	".zsh": kShell, ".fish": kShell, ".lua": kLua, ".md": kMarkdown, ".mdx": kMarkdown, ".json": kJSON,
	".jsonc": kJSON, ".yml": kConfig, ".yaml": kConfig, ".toml": kConfig, ".ini": kConfig, ".cfg": kConfig,
	".html": kHTML, ".htm": kHTML, ".css": kCSS, ".scss": kSass, ".sass": kSass, ".less": kCSS,
	".sql": kSQL, ".lock": kLock, ".sum": kLock, ".png": kImage, ".jpg": kImage, ".jpeg": kImage,
	".gif": kImage, ".svg": kImage, ".webp": kImage, ".ico": kImage, ".bmp": kImage, ".txt": kText,
	".mk": kMake, ".mod": kGo, ".env": kEnv,
}

// fileNames maps a lower-case file name to how it is drawn; it wins over the extension.
var fileNames = map[string]fileKind{
	"dockerfile": kDocker, "makefile": kMake, "gnumakefile": kMake, "cmakelists.txt": kMake,
	"go.mod": kGo, "go.sum": kLock, "package.json": kNpm, "package-lock.json": kLock,
	"yarn.lock": kLock, "pnpm-lock.yaml": kLock, "cargo.lock": kLock, "gemfile.lock": kLock,
	".gitignore": kGit, ".gitattributes": kGit, ".gitmodules": kGit, ".env": kEnv,
	"license": kLicense, "licence": kLicense, "readme": kMarkdown, "readme.md": kMarkdown,
	"docker-compose.yml": kDocker, "docker-compose.yaml": kDocker, "compose.yml": kDocker, "compose.yaml": kDocker,
}

// kindOf is the kind a path is drawn as.
func kindOf(p string) fileKind {
	name := strings.ToLower(path.Base(strings.ReplaceAll(p, "\\", "/")))
	if k, ok := fileNames[name]; ok {
		return k
	}
	switch {
	case strings.HasPrefix(name, "dockerfile."), strings.HasPrefix(name, "docker-compose."):
		return kDocker
	case strings.HasPrefix(name, ".env."):
		return kEnv
	case strings.HasSuffix(name, ".d.ts"):
		return kTS
	}
	if strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") || strings.HasSuffix(name, "_test.go") {
		return kTest
	}
	if ext := path.Ext(name); ext != "" && ext != name { // a dotfile alone (".hidden") has no extension
		if k, ok := fileKinds[ext]; ok {
			return k
		}
	}
	return kGeneric
}

// FileIcon is the glyph and colour of a file by language or kind; both empty in the plain set.
func FileIcon(p string) (glyph, color string) {
	if Current() != Nerd {
		return "", ""
	}
	k := kindOf(p)
	return k.glyph, k.color
}

// FolderIcon is the glyph and colour of a directory; both empty in the plain set.
func FolderIcon(open bool) (glyph, color string) {
	if Current() != Nerd {
		return "", ""
	}
	if open {
		return "", "#7AA2F7" // nf-fa-folder_open
	}
	return "", "#7AA2F7" // nf-fa-folder
}
