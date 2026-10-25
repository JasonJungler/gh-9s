#!/bin/sh
# Puts a motion recording and its GH9S_MOTION=off control side by side: docs/demo/compare-NAME.{mp4,gif}.
# Usage: docs/demo/compare.sh motion-screens     (needs NAME.mp4 and NAME-off.mp4, i.e. record.sh run on both tapes)
# The off tape types a longer command line, so its start is trimmed by the difference in length; the labels are
# drawn with swift (macOS), as this ffmpeg has no drawtext.
set -e
here=$(cd "$(dirname "$0")" && pwd)
n=${1:?name, e.g. motion-screens}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
dur() { ffprobe -v error -show_entries format=duration -of csv=p=0 "$1"; }
skew=$(echo "$(dur "$here/$n-off.mp4") - $(dur "$here/$n.mp4")" | bc -l)
w=$(ffprobe -v error -select_streams v -show_entries stream=width -of csv=p=0 "$here/$n.mp4")
cat > "$tmp/label.swift" <<'S'
import AppKit
let a = CommandLine.arguments
let text = a[1], out = a[2], w = Int(a[3])!, hex = a[4]
let h = 48
let img = NSImage(size: NSSize(width: w, height: h))
img.lockFocus()
let v = UInt32(hex, radix: 16)!
NSColor(red: CGFloat((v>>16)&255)/255, green: CGFloat((v>>8)&255)/255, blue: CGFloat(v&255)/255, alpha: 1).setFill()
NSRect(x: 0, y: 0, width: w, height: h).fill()
let attrs: [NSAttributedString.Key: Any] = [.font: NSFont.boldSystemFont(ofSize: 26), .foregroundColor: NSColor.white]
let s = NSAttributedString(string: text, attributes: attrs)
let sz = s.size()
s.draw(at: NSPoint(x: (CGFloat(w)-sz.width)/2, y: (CGFloat(h)-sz.height)/2))
img.unlockFocus()
let rep = NSBitmapImageRep(data: img.tiffRepresentation!)!
try! rep.representation(using: .png, properties: [:])!.write(to: URL(fileURLWithPath: out))
S
swift "$tmp/label.swift" "Motion on" "$tmp/on.png" "$w" 0f6cbd
swift "$tmp/label.swift" "Motion off (GH9S_MOTION=off)" "$tmp/off.png" "$w" 6e6e6e
ffmpeg -v error -y -i "$here/$n.mp4" -ss "$skew" -i "$here/$n-off.mp4" -i "$tmp/on.png" -i "$tmp/off.png" -filter_complex \
  "[2:v]scale=$w:48[a1];[3:v]scale=$w:48[b1];[a1][0:v]vstack[l];[b1][1:v]vstack[r];[l][r]hstack=shortest=1,format=yuv420p[v]" -map "[v]" "$here/compare-$n.mp4"
ffmpeg -v error -y -i "$here/compare-$n.mp4" -vf "fps=15,scale=1600:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=96[p];[b][p]paletteuse=dither=bayer:bayer_scale=4" "$here/compare-$n.gif"
