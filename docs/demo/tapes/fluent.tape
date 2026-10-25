# The Fluent-motion experiment (cmd/fluent-demo): tabs slide, cards fade. Needs no pretend gh.
Output fluent.gif
Set Shell "bash"
Set FontSize 15
Set Width 760
Set Height 480
Set Padding 14
Set Theme "Catppuccin Mocha"
Set TypingSpeed 70ms
Set Framerate 50
Set WindowBar Colorful
Set BorderRadius 8

Hide
Type 'clear'
Enter
Type '$DEMO/fluent-demo'
Enter
Sleep 1s
Show
Sleep 1s

# cards fade as the selection moves
Type "j"
Sleep 700ms
Type "j"
Sleep 700ms
Type "k"
Sleep 900ms

# tabs slide, the underline follows
Type "l"
Sleep 1s
Type "j"
Sleep 700ms
Type "l"
Sleep 1s
Type "h"
Sleep 1s
Type "h"
Sleep 1500ms
