#!/bin/sh
set -eu

fail() { printf '%s\n' "$*" >&2; exit 1; }

repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
cd "$repo"
branch=$(git symbolic-ref --quiet --short HEAD) || fail 'Detached HEAD: install from jai/chat-rich-messages.'
[ "$branch" = jai/chat-rich-messages ] || fail "Unexpected branch: $branch"
origin=$(git remote get-url origin)
case "$origin" in
    git@github.com:jai/gogcli.git|https://github.com/jai/gogcli.git|https://github.com/jai/gogcli) ;;
    *) fail "Unexpected origin: $origin" ;;
esac
[ -z "$(git status --porcelain --untracked-files=all)" ] || fail 'Source has tracked or untracked changes; commit before installation.'

sha=$(git rev-parse HEAD)
short_sha=$(git rev-parse --short=12 HEAD)
base_version=$(git describe --tags --abbrev=0)
version="$base_version-jai-chat.$short_sha"
build_root="$HOME/.local/share/gogcli-fork/builds"
build_dir="$build_root/$sha"
active="$HOME/.local/bin/gog"
homebrew_original=/opt/homebrew/Cellar/gogcli/0.43.0/bin/gog
previous=absent
[ ! -d "$active" ] || fail "Existing local gog resolves to a directory; preserved: $active"

if [ -L "$active" ]; then
    previous=$(readlink "$active")
    case "$previous" in
        "$homebrew_original") ;;
        "$build_root"/*/gog)
            prior_sha=${previous#"$build_root/"}
            prior_sha=${prior_sha%/gog}
            case "$prior_sha" in ''|*[!0-9a-f]*) fail "Unknown local gog target preserved: $previous" ;; esac
            [ "${#prior_sha}" -eq 40 ] && [ -f "$build_root/$prior_sha/provenance.txt" ] || fail "Unknown local gog target preserved: $previous"
            grep -Fx 'source_repository=https://github.com/jai/gogcli' "$build_root/$prior_sha/provenance.txt" >/dev/null || fail "Unknown local gog target preserved: $previous"
            ;;
        *) fail "Unknown local gog target preserved: $previous" ;;
    esac
elif [ -e "$active" ]; then
    fail "Existing local gog is not a recognized symlink; preserved: $active"
fi
printf 'Previous local gog: %s\n' "$previous"

mkdir -p "$build_root" "$HOME/.local/bin"
stage=
switch_dir=
cleanup() {
    [ -z "$stage" ] || rm -rf "$stage"
    [ -z "$switch_dir" ] || rm -rf "$switch_dir"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM

check_source_unchanged() {
    [ "$(git rev-parse HEAD)" = "$sha" ] && [ "$(git symbolic-ref --quiet --short HEAD)" = "$branch" ] && [ -z "$(git status --porcelain --untracked-files=all)" ] || fail 'Source changed during build; active binary was not switched.'
}

if [ -e "$build_dir" ] || [ -L "$build_dir" ]; then
    [ -d "$build_dir" ] && [ ! -L "$build_dir" ] && [ -f "$build_dir/provenance.txt" ] || fail "Unrecognized build directory preserved: $build_dir"
    grep -Fx "source_sha=$sha" "$build_dir/provenance.txt" >/dev/null || fail 'Existing build provenance does not match source.'
    grep -Fx 'source_repository=https://github.com/jai/gogcli' "$build_dir/provenance.txt" >/dev/null || fail 'Existing build provenance does not identify the fork.'
else
    stage=$(mktemp -d "$build_root/.stage.XXXXXX")
    make build "BIN=$stage/gog" "VERSION=$version" "COMMIT=$short_sha"
    [ -x "$stage/gog" ] || fail 'Build did not produce an executable.'
    built_version=$("$stage/gog" --version)
    case "$built_version" in *"$version"*) ;; *) fail "Unexpected built version: $built_version" ;; esac
    {
        printf 'source_repository=https://github.com/jai/gogcli\nsource_branch=%s\nsource_sha=%s\nversion=%s\n' "$branch" "$sha" "$version"
        printf 'source_checkout=%s\nhomebrew_original=%s\nprevious_local_target=%s\n' "$repo" "$homebrew_original" "$previous"
    } > "$stage/provenance.txt"
    check_source_unchanged
    mv "$stage" "$build_dir"
    stage=
fi

[ -x "$build_dir/gog" ] || fail 'Installed build is not executable.'
installed_version=$("$build_dir/gog" --version)
case "$installed_version" in *"$version"*) ;; *) fail "Unexpected installed version: $installed_version" ;; esac
check_source_unchanged
if [ "$previous" = absent ]; then
    [ ! -e "$active" ] && [ ! -L "$active" ] || fail 'Local gog appeared during build; preserved without switching.'
else
    [ -L "$active" ] && [ "$(readlink "$active")" = "$previous" ] && [ ! -d "$active" ] || fail 'Local gog changed during build; preserved without switching.'
fi

switch_dir=$(mktemp -d "$HOME/.local/bin/.gog-switch.XXXXXX")
ln -s "$build_dir/gog" "$switch_dir/gog"
mv -f "$switch_dir/gog" "$active"
printf 'Active gog: %s\nVersion: %s\nProvenance: %s\nHomebrew preserved: %s\n' "$build_dir/gog" "$installed_version" "$build_dir/provenance.txt" "$homebrew_original"
