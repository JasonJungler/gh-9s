# Fluent-style motion

## Intent
gh-9s should feel as fluid as a Fluent UI app (sliding tabs, cards that light up) without giving up the dense,
k9s-style screens it has. Motion is transient: it never adds a row, a column or a key, and it is gone after
about a fifth of a second.

## What moves
- **Between screens**: opening a screen pushes the body in from the right, going back pops it from the left
  (220 ms, Fluent "decelerate"). The two header lines stay put.
- **Tabs** (Favorites, Project board Status tabs and views): the content below the strip slides in the direction of
  the tab (260 ms), and the highlighted tab slides along the strip, resizing to the new label.
- **Selection**: the selected table row or board card fades in as the cursor arrives and the old one fades out
  (180 ms); a fade turned around half-way continues from where it is. On the board columns every card is a
  slightly tinted block that lights up.
- **Dialogs, pickers, forms, help**: rise three lines into place (160 ms).
- **Loading**: a braille spinner before "loading…".

## Rules
- `GH9S_MOTION=off` turns all of it off. Without truecolor the slides and the tab highlight still run; the fades
  (which blend colours) and the tinted cards do not, and the usual reverse-video selection stays.
- Frames (16 ms) only run while something moves; an idle screen redraws nothing extra.
- No change to what is shown at rest, apart from the tinted selection on truecolor terminals.

## How
`internal/anim` holds the easing curves, tweens that can be turned around, and colour mixing. `internal/tui/motion.go`
wraps `View`: it keeps the last frame drawn, notices when the screen or tab changed, and draws the frame between the
two. State that must outlive a frame sits behind one pointer on the model; a model without it (tests) never animates.

## Not done
Mouse-wheel and scroll-position easing, animated log following, and a switch in the Settings screen.
