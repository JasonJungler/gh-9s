# Control group: the same keys as board-motion.tape with GH9S_MOTION=off.
Output board-motion-off.gif
Output board-motion-off.mp4
Source tapes/common.tape
Set Framerate 30

Type "GH9S_MOTION=off COLORTERM=truecolor gh-9s"
Enter
Sleep 2s
Type ":"
Sleep 300ms
Type "fav"
Enter
Sleep 1500ms
# the Favorites tabs slide
Type "l"
Sleep 900ms
Type "l"
Sleep 1200ms
Enter
Sleep 2500ms

# the board as a list: rows fade, Status tabs slide
Type "j"
Sleep 800ms
Type "j"
Sleep 800ms
Type "l"
Sleep 1100ms
Type "l"
Sleep 1100ms
Type "h"
Sleep 1100ms
# views slide
Type "]"
Sleep 1200ms
Type "["
Sleep 1200ms

# the board as columns: cards fade
Type "b"
Sleep 1500ms
Type "j"
Sleep 800ms
Type "l"
Sleep 900ms
Type "j"
Sleep 800ms
Type "h"
Sleep 800ms
Type "k"
Sleep 1500ms
