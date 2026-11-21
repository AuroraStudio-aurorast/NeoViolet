#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
shopt -s nocasematch

fail() {
    echo "error: $*" >&2
    exit 1
}

lower() {
    printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

# Resolve existing ancestors, including symlinks, without creating the output.
canonical_dir() {
    local path="$1" parent leaf
    if [[ -d "$path" ]]; then
        (cd "$path" && pwd -P)
        return
    fi
    [[ ! -e "$path" && ! -L "$path" ]] || fail "not a directory: $path"
    parent="$(canonical_dir "$(dirname "$path")")" || return
    leaf="$(basename "$path")"
    case "$leaf" in
        .) printf '%s\n' "$parent" ;;
        ..) dirname "$parent" ;;
        *) printf '%s/%s\n' "${parent%/}" "$leaf" ;;
    esac
}

canonical_file() {
    local path="$1" target parent
    while [[ -L "$path" ]]; do
        target="$(readlink "$path")" || return
        case "$target" in
            /*) path="$target" ;;
            *) path="$(dirname "$path")/$target" ;;
        esac
    done
    parent="$(canonical_dir "$(dirname "$path")")" || return
    printf '%s/%s\n' "${parent%/}" "$(basename "$path")"
}

# Replacing a child of the input (such as input/dist) is safe; replacing a
# source directory or any of its ancestors would remove the source artifacts.
check_source() {
    local source
    source="$(lower "$1")"
    if [[ "$output_lower" == / || "$source" == "$output_lower" || "$source" == "$output_lower/"* ]]; then
        fail "output directory contains a source: $1"
    fi
}

: "${PACKAGE_INPUT_DIR:?missing PACKAGE_INPUT_DIR}"
: "${PACKAGE_OUTPUT_DIR:?missing PACKAGE_OUTPUT_DIR}"
: "${WINDOWS_DLL_DIR:?missing WINDOWS_DLL_DIR}"
: "${LIBOPENMPT_DLL:?missing LIBOPENMPT_DLL}"
OBJDUMP="${OBJDUMP:-objdump}"
INPUT_DIR="$(cd "$PACKAGE_INPUT_DIR" && pwd -P)"
DLL_DIR="$(cd "$WINDOWS_DLL_DIR" && pwd -P)"
output="$PACKAGE_OUTPUT_DIR"
while [[ "$output" != / ]]; do
    case "$output" in
        */) output="${output%/}" ;;
        */.) output="${output%/.}"; output="${output:-/}" ;;
        *) break ;;
    esac
done
[[ ! -L "$output" ]] || fail "output directory must not be a symlink: $output"
OUTPUT_DIR="$(canonical_dir "$output")"
output_lower="$(lower "$OUTPUT_DIR")"
check_source "$INPUT_DIR"
check_source "$DLL_DIR"

for bin in neoviolet.exe neoviolet-gui.exe apecli.exe; do
    [[ -f "$INPUT_DIR/$bin" ]] || fail "missing input: $INPUT_DIR/$bin"
    check_source "$(canonical_file "$INPUT_DIR/$bin")"
done
[[ -f "$LIBOPENMPT_DLL" ]] || fail "missing libopenmpt: $LIBOPENMPT_DLL"
LIBOPENMPT_DLL="$(canonical_file "$LIBOPENMPT_DLL")"
check_source "$LIBOPENMPT_DLL"

mkdir -p "$(dirname "$OUTPUT_DIR")"
tmp="$(mktemp -d "$(dirname "$OUTPUT_DIR")/.neoviolet-windows.XXXXXX")"
cleanup() {
    status=$?
    trap - EXIT
    if [[ -e "$tmp/old" && ! -e "$OUTPUT_DIR" ]]; then
        if ! mv "$tmp/old" "$OUTPUT_DIR"; then
            echo "error: could not restore output; previous distribution is at $tmp/old" >&2
            exit 1
        fi
    fi
    rm -rf "$tmp"
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
STAGE="$tmp/new"
mkdir "$STAGE"
pending=()
for bin in neoviolet.exe neoviolet-gui.exe apecli.exe; do
    cp "$INPUT_DIR/$bin" "$STAGE/$bin"
    pending+=("$STAGE/$bin")
done
cp "$LIBOPENMPT_DLL" "$STAGE/libopenmpt.dll"
pending+=("$STAGE/libopenmpt.dll")

# Keep the Windows-provided imports in sync with the supported CI platforms.
SYSTEM_DLLS='^(kernel32|ntdll|user32|gdi32|advapi32|shell32|ole32|msvcrt|vcruntime|'
SYSTEM_DLLS+='(api|ext)-ms-win-[a-z0-9-]*|ucrtbase|ws2_32|iphlpapi|setupapi|crypt32|bcrypt|'
SYSTEM_DLLS+='bcryptprimitives|psapi|winmm|version|combase|rpcrt4|shlwapi|oleaut32|imm32|msctf|'
SYSTEM_DLLS+='comctl32|uxtheme|opengl32|d3d11|dcomp|dwmapi|dwrite|dxgi|icu(uc|in)?|kernel\.appcore|windows\.storage)\.dll$'

find_dll() {
    local dir candidate
    for dir in "$INPUT_DIR" "$DLL_DIR"; do
        for candidate in "$dir"/*; do
            if [[ -f "$candidate" && "${candidate##*/}" == "$1" ]]; then
                printf '%s\n' "$candidate"
                return
            fi
        done
    done
    return 1
}

seen='|libopenmpt.dll|'
missing=0
index=0
while [[ "$index" -lt "${#pending[@]}" ]]; do
    current="${pending[$index]}"
    index=$((index + 1))
    imports="$("$OBJDUMP" -p "$current")" || fail "objdump failed: $current"
    while IFS= read -r line; do
        line="$(lower "$line")"
        case "$line" in
            *'dll name:'*) dll="${line#*dll name:}" ;;
            *) continue ;;
        esac
        dll="$(printf '%s' "$dll" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
        [[ "$dll" =~ ^[a-z0-9_][a-z0-9_.+-]*\.dll$ ]] || fail "invalid DLL name: $dll"
        if [[ "$dll" =~ $SYSTEM_DLLS || "$seen" == *"|$dll|"* ]]; then
            continue
        fi
        seen+="$dll|"
        if ! source="$(find_dll "$dll")"; then
            echo "error: missing non-system DLL: $dll (imported by $current)" >&2
            missing=1
            continue
        fi
        check_source "$(canonical_file "$source")"
        cp "$source" "$STAGE/$dll"
        pending+=("$STAGE/$dll")
    done <<< "$imports"
done

[[ "$missing" -eq 0 ]] || fail "DLL dependency check failed; distribution was not replaced"

if [[ -e "$OUTPUT_DIR" ]]; then
    mv "$OUTPUT_DIR" "$tmp/old"
fi
mv "$STAGE" "$OUTPUT_DIR"
echo "$OUTPUT_DIR"
