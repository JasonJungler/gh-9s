# Demo GIFs

The GIFs in the README are recorded with [VHS](https://github.com/charmbracelet/vhs) against a pretend `gh`
(`bin/gh`, answering from `fixtures.py`): an imaginary repo `acme/app`, no network, no real account, and a
throwaway home directory, so nothing real is shown or changed.

    brew install vhs
    docs/demo/record.sh                      # all tapes
    docs/demo/record.sh tapes/board.tape     # one

Edit `fixtures.py` for the data and `tapes/*.tape` for the key presses. Project changes made while recording are
kept in a state file inside the temporary directory and vanish with it.
