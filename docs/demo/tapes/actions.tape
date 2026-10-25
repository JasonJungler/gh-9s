Output actions.gif
Source tapes/common.tape

Type "gh-9s"
Enter
Sleep 2500ms

# the workflows of the repo
Type "4"
Sleep 2s
Type "3"
Sleep 1500ms

# a failed run as a live job graph
Down
Sleep 800ms
Enter
Sleep 3500ms

# walk to the job that failed and read its log
Type "j"
Sleep 600ms
Type "l"
Sleep 1s
Enter
Sleep 3500ms
Escape
Sleep 800ms
Escape
Sleep 1s

# run the failed jobs again: asks first
Down
Sleep 800ms
Type "F"
Sleep 3s
Escape
Sleep 1s
Type "q"
Sleep 500ms
