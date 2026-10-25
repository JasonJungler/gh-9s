#!/bin/sh
# Prepares a throwaway home for recording: no real settings, favorites or tokens are read.
# Usage: eval "$(docs/demo/setup.sh DIR)"   (prints the exports to use)
set -e
here=$(cd "$(dirname "$0")" && pwd)
dir=${1:?directory}
rm -rf "$dir"
cfg="$dir/home/Library/Application Support/gh-9s"
mkdir -p "$cfg" "$dir/home/.config/gh-9s"
for c in "$cfg" "$dir/home/.config/gh-9s"; do
  echo '{"icons":"plain"}' > "$c/settings.json"
  echo '{"repos":[{"host":"github.com","repo":"acme/app"}],"projects":[{"host":"github.com","owner":"acme","number":1,"title":"Sprint board"}]}' > "$c/pins.json"
  echo '[{"host":"github.com","repo":"acme/app","workflow":"ci.yml"},{"host":"github.com","repo":"acme/app","workflow":"release.yml"}]' > "$c/favorites.json"
done
git init -q -b main "$dir/work"
git -C "$dir/work" -c user.name=mira -c user.email=mira@example.com commit -q --allow-empty -m init
echo "export HOME='$dir/home' GH_FAKE_STATE='$dir/state.json' GH_FAKE_LOG='$dir/log' PATH='$here/bin':\$PATH GH9S_ICONS=plain"
