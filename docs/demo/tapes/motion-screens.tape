# Fluent motion between screens: push and pop slide, rows fade, a dialog rises (truecolor).
Output motion-screens.gif
Output motion-screens.mp4
Source tapes/common.tape
Set Framerate 30

Type "COLORTERM=truecolor gh-9s"
Enter
Sleep 2s

# rows fade as the cursor moves; screens push sideways
Type "4"
Sleep 1500ms
Type "j"
Sleep 700ms
Type "j"
Sleep 700ms
Type "k"
Sleep 900ms
Enter
Sleep 1800ms
Escape
Sleep 1500ms
Type "3"
Sleep 1500ms
Type "j"
Sleep 800ms
Enter
Sleep 1800ms
Escape
Sleep 1200ms
Type "1"
Sleep 1200ms

# a dialog rises
Type "?"
Sleep 2s
Escape
Sleep 1s
Type "q"
Sleep 500ms
