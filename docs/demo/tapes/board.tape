Output board.gif
Source tapes/common.tape

Type "gh-9s"
Enter
Sleep 2s

# pinned Projects live in :fav
Type ":"
Sleep 300ms
Type "fav"
Enter
Sleep 2s
Type "l"
Sleep 400ms
Type "l"
Sleep 1500ms
Enter
Sleep 3s

# a real board: one column per Status
Type "b"
Sleep 3s

# move a card: asks first, says exactly what changes
Type "L"
Sleep 2500ms
Type "y"
Sleep 3s

# changed your mind: undo asks first as well
Type "u"
Sleep 2500ms
Type "y"
Sleep 3s

# mark two cards, move both at once
Type "h"
Sleep 800ms
Space
Sleep 600ms
Type "j"
Sleep 400ms
Space
Sleep 1500ms
Type "s"
Sleep 2s
Down 3
Sleep 600ms
Enter
Sleep 2500ms
Type "y"
Sleep 3500ms
