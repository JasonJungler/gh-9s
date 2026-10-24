# Syntax highlighting in diffs — plan

**Goal:** diff lines (side-by-side and unified) are coloured by language, using chroma, per hunk (the user chose the no-extra-requests approximation).

**Design (decided with the user):** a new `internal/highlight` package tokenises each hunk's old side (context + deleted lines) and new side (context + added lines) with chroma, so multi-line strings and comments inside a hunk are right; a construct that starts outside the hunk may be coloured wrongly. Colours come from one chroma style and are given to lipgloss as hex, which lipgloss reduces to the terminal's colour depth (so macOS Terminal gets 256 colours). Added/deleted lines keep a dark green/red background tint (the sign and line number too) with syntax foreground on top; changed words get a stronger background. Unknown languages, very long lines (> 2,000 chars), and diffs beyond 20,000 highlighted lines fall back to today's plain add/del colours. Highlighting is computed once per fetched diff (not per frame/resize).

**Tasks:** (1) `internal/highlight` + tests; (2) side-by-side cells use it; (3) unified rows use it; (4) README. TDD, mutation-check, one fresh review, then the merge menu.
