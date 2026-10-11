#!/usr/bin/env bash
set -euo pipefail
version="${1:-}"
destination="${TELDRIVE_INSTALL_DIR:-$HOME/.local/bin}"
for tool in curl tar sha256sum; do command -v "$tool" >/dev/null || { echo "Required tool: $tool" >&2; exit 1; }; done
[[ "$(uname -s)" == Linux ]] || { echo 'This installer supports Linux only.' >&2; exit 1; }
case "$(uname -m)" in
  x86_64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  armv7l) arch=arm ;;
  *) echo 'Unsupported architecture.' >&2; exit 1 ;;
esac
if [[ -z "$version" ]]; then
  command -v python3 >/dev/null || { echo 'Auto-detection requires python3; alternatively pass a version.' >&2; exit 1; }
  version="$(curl -fsSL https://api.github.com/repos/tgdrive/teldrive/releases/latest | python3 -c 'import json,sys; r=json.load(sys.stdin); assert not r.get("draft") and not r.get("prerelease"); print(r["tag_name"])')"
fi
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ && "$version" != 0.* && "$version" != 1.* ]] || { echo 'A stable Teldrive 2.x or newer release is required.' >&2; exit 1; }
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
asset="teldrive-$version-linux-$arch.tar.gz"
base="https://github.com/tgdrive/teldrive/releases/download/$version"
curl -fsSL "$base/$asset" -o "$work/$asset"
curl -fsSL "$base/teldrive_checksums.txt" -o "$work/checksums"
awk -v name="$asset" '$2 == name { print }' "$work/checksums" > "$work/expected"
[[ "$(wc -l < "$work/expected")" -eq 1 ]] || { echo 'Missing or ambiguous checksum.' >&2; exit 1; }
(cd "$work"; sha256sum -c expected)
tar -xzf "$work/$asset" -C "$work" teldrive completions
mkdir -p "$destination"
install -m 0755 "$work/teldrive" "$destination/.teldrive-new"
mv -f "$destination/.teldrive-new" "$destination/teldrive"
data="${XDG_DATA_HOME:-$HOME/.local/share}"
mkdir -p "$data/bash-completion/completions" "$data/zsh/site-functions" "$HOME/.config/fish/completions"
cp "$work/completions/teldrive.bash" "$data/bash-completion/completions/teldrive"
cp "$work/completions/teldrive.zsh" "$data/zsh/site-functions/_teldrive"
cp "$work/completions/teldrive.fish" "$HOME/.config/fish/completions/teldrive.fish"
echo "Installed Teldrive $version to $destination/teldrive. Add $destination to PATH if needed."
echo "Zsh: add $data/zsh/site-functions to fpath before compinit."
