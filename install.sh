#!/usr/bin/env bash
# Self-contained macOS installer: Go, uv, Python, NumPy/SciPy, Rubber Band.
# No Homebrew, Xcode CLT, global Python, or administrator access needed.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tools_dir="$repo_root/.cusic-tools"
bin_dir="$repo_root/bin"
mkdir -p "$tools_dir" "$bin_dir"

case "$(uname -s):$(uname -m)" in
  Darwin:arm64)
    go_archive="go1.26.10.darwin-arm64.tar.gz"
    go_sha="0b5183492292ada170f9b561a834863808dd02edba599e4ad0e89899330020b3"
    uv_archive="uv-aarch64-apple-darwin.tar.gz"
    uv_sha="81b811b57e4624df7ed3aac148e0359100d9962f3d9dce00aec533dc2e086731"
    ;;
  Darwin:x86_64)
    go_archive="go1.26.10.darwin-amd64.tar.gz"
    go_sha="e820ab780c273ae4a340b79d0f3c4313b74ee029aa3874792145d44b9f4f91da"
    uv_archive="uv-x86_64-apple-darwin.tar.gz"
    uv_sha="20c47e4c1f630b8abaff8c291fa9b9aa300fef92c4fbff95adf198b55287cda7"
    ;;
  *)
    echo "Unsupported platform. macOS Apple Silicon and Intel x86_64 only." >&2
    exit 1
    ;;
esac

download_verified() {
  local url="$1" expected="$2" destination="$3" actual
  curl --fail --location --retry 3 --silent --show-error "$url" --output "$destination"
  actual="$(shasum -a 256 "$destination" | awk '{print $1}')"
  if [[ "$actual" != "$expected" ]]; then
    echo "SHA-256 mismatch for $url (got $actual); refusing to install." >&2
    return 1
  fi
}

scratch="$(mktemp -d "${TMPDIR:-/tmp}/cusic-install.XXXXXX")"
trap 'rm -rf "$scratch"' EXIT

go_exe="$tools_dir/go/bin/go"
if [[ ! -x "$go_exe" ]]; then
  if [[ -e "$tools_dir/go" ]]; then
    echo "Incomplete portable Go install: remove .cusic-tools/go and retry." >&2
    exit 1
  fi
  echo 'Installing portable Go 1.26.10...'
  download_verified "https://go.dev/dl/$go_archive" "$go_sha" "$scratch/go.tgz"
  mkdir -p "$scratch/go-extract"
  tar -xzf "$scratch/go.tgz" -C "$scratch/go-extract"
  [[ -x "$scratch/go-extract/go/bin/go" ]] || { echo 'Missing Go executable' >&2; exit 1; }
  mv "$scratch/go-extract/go" "$tools_dir/go"
fi

export GOROOT="$tools_dir/go"
export GOTOOLCHAIN=local
echo 'Building cusic with the bundled Go runtime...'
(cd "$repo_root" && "$go_exe" build -mod=readonly -o "$bin_dir/cusic" ./cmd/cusic)

uv_exe="$tools_dir/uv/uv"
if [[ ! -x "$uv_exe" ]]; then
  echo 'Installing private uv 0.13.7...'
  download_verified "https://github.com/astral-sh/uv/releases/download/0.13.7/$uv_archive" "$uv_sha" "$scratch/uv.tgz"
  mkdir -p "$scratch/uv-extract"
  tar -xzf "$scratch/uv.tgz" -C "$scratch/uv-extract"
  uv_found="$(find "$scratch/uv-extract" -type f -name uv | head -n 1)"
  [[ -n "$uv_found" ]] || { echo 'Missing uv executable' >&2; exit 1; }
  mkdir -p "$tools_dir/uv"
  cp "$uv_found" "$uv_exe"
  chmod +x "$uv_exe"
fi

export UV_PYTHON_INSTALL_DIR="$tools_dir/python"
export UV_CACHE_DIR="$tools_dir/cache"
export UV_PYTHON_PREFERENCE=only-managed
export UV_NO_PROGRESS=1
python_exe="$tools_dir/venv/bin/python"
if [[ ! -x "$python_exe" ]]; then
  echo 'Installing private Python 3.12...'
  "$uv_exe" python install 3.12
  "$uv_exe" venv --python 3.12 --python-preference only-managed "$tools_dir/venv"
fi
echo 'Installing NumPy and SciPy inside cusic private Python venv...'
"$uv_exe" pip install --python "$python_exe" -r "$repo_root/audio/requirements.txt"
"$python_exe" -c 'import numpy, scipy; print("NumPy/SciPy ready:", numpy.__version__, scipy.__version__)'

if [[ ! -x "$bin_dir/rubberband" ]]; then
  if command -v rubberband >/dev/null 2>&1; then
    echo 'Rubber Band is already available on PATH.'
  else
    echo 'Installing Rubber Band 4.0.0 from official macOS distribution...'
    download_verified "https://breakfastquay.com/files/releases/rubberband-4.0.0-gpl-executable-macos.tar.bz2" "__MAC_RUBBERBAND_SHA256__" "$scratch/rubberband.tbz"
    mkdir -p "$scratch/rubberband-extract"
    tar -xjf "$scratch/rubberband.tbz" -C "$scratch/rubberband-extract"
    rubberband_found="$(find "$scratch/rubberband-extract" -type f -name rubberband | head -n 1)"
    [[ -n "$rubberband_found" ]] || { echo 'Missing rubberband CLI in official archive' >&2; exit 1; }
    distribution_dir="$(dirname "$rubberband_found")"
    for source in "$distribution_dir"/*; do
      [[ -f "$source" ]] || continue
      [[ "$(basename "$source")" != 'cusic' ]] || { echo 'Unexpected cusic binary in archive' >&2; exit 1; }
      cp "$source" "$bin_dir/$(basename "$source")"
    done
    chmod +x "$bin_dir/rubberband"
  fi
fi

# zsh is macOS's default shell; also support bash login shells.
# Keep the existing user PATH and avoid duplicate profile entries.
escaped_bin="$(printf '%q' "$bin_dir")"
profile_line="export PATH=$escaped_bin:\$PATH"
for profile in "$HOME/.zshrc" "$HOME/.bash_profile"; do
  touch "$profile"
  if ! grep -Fqx "$profile_line" "$profile"; then
    printf '\n# cusic CLI\n%s\n' "$profile_line" >> "$profile"
  fi
done

echo 'cusic, isolated Go/Python, NumPy/SciPy and Rubber Band installed!'
echo 'Open a new terminal and run: cusic help'
