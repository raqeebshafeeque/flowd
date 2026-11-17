#!/bin/sh
# CI/developer tools only; not an end-user installation dependency.
set -eu
[ "$#" -eq 1 ] || { printf '%s\n' 'usage: ci-tools.sh DIRECTORY' >&2; exit 2; }
destination=$1
mkdir -p "$destination"
destination=$(CDPATH='' cd "$destination" && pwd -P)
case "$(uname -s)" in Darwin) platform=darwin; goreleaser_os=Darwin ;; Linux) platform=linux; goreleaser_os=Linux ;; *) exit 2 ;; esac
case "$(uname -m)" in arm64|aarch64) arch=arm64; long_arch=aarch64; goreleaser_arch=arm64 ;; amd64|x86_64) arch=amd64; long_arch=x86_64; goreleaser_arch=x86_64 ;; *) exit 2 ;; esac
staging=$(mktemp -d "${TMPDIR:-/tmp}/flowd-ci-tools.XXXXXX")
trap 'rm -rf "$staging"' EXIT

for tool in goreleaser actionlint shellcheck; do
    case "$tool" in
        goreleaser)
            version=2.20.0
            url=https://github.com/goreleaser/goreleaser/releases/download/v$version/goreleaser_${goreleaser_os}_${goreleaser_arch}.tar.gz
            member=goreleaser ;;
        actionlint)
            version=1.7.12
            url=https://github.com/rhysd/actionlint/releases/download/v$version/actionlint_${version}_${platform}_${arch}.tar.gz
            member=actionlint ;;
        shellcheck)
            version=0.11.0
            url=https://github.com/koalaman/shellcheck/releases/download/v$version/shellcheck-v${version}.${platform}.${long_arch}.tar.gz
            member=shellcheck-v$version/shellcheck ;;
    esac
    case "$tool/$platform/$arch" in
        goreleaser/darwin/arm64) expected=50bd83a6fcbf1dac272cfc4c46f9116052a9b0b463404a044af71a832fd92ad6 ;;
        goreleaser/darwin/amd64) expected=a1dfe9492c67b860129bc08fa5952955f0666abe4741e1846f93a6c6b47360e6 ;;
        goreleaser/linux/arm64) expected=2dada0a6bbcd10f3a05228d98e77d25fb362efb9e5bab6362cf37e64c8a26cb9 ;;
        goreleaser/linux/amd64) expected=d2483159e6007f814132cb56635ad30a70013dbeb813d6033a7e01adf6ae108c ;;
        actionlint/darwin/arm64) expected=aba9ced2dee8d27fecca3dc7feb1a7f9a52caefa1eb46f3271ea66b6e0e6953f ;;
        actionlint/darwin/amd64) expected=5b44c3bc2255115c9b69e30efc0fecdf498fdb63c5d58e17084fd5f16324c644 ;;
        actionlint/linux/arm64) expected=325e971b6ba9bfa504672e29be93c24981eeb1c07576d730e9f7c8805afff0c6 ;;
        actionlint/linux/amd64) expected=8aca8db96f1b94770f1b0d72b6dddcb1ebb8123cb3712530b08cc387b349a3d8 ;;
        shellcheck/darwin/arm64) expected=339b930feb1ea764467013cc1f72d09cd6b869ebf1013296ba9055ab2ffbd26f ;;
        shellcheck/darwin/amd64) expected=c2c15e08df0e8fbc374c335b230a7ee958c313fa5714817a59aa59f1aa594f51 ;;
        shellcheck/linux/arm64) expected=68a8133197a50beb8803f8d42f9908d1af1c5540d4bb05fdfca8c1fa47decefc ;;
        shellcheck/linux/amd64) expected=b7af85e41cc99489dcc21d66c6d5f3685138f06d34651e6d34b42ec6d54fe6f6 ;;
        *) printf '%s\n' 'unsupported CI tool target' >&2; exit 2 ;;
    esac
    curl --proto '=https' --proto-redir '=https' -fsSL --retry 2 --connect-timeout 10 --max-time 180 "$url" -o "$staging/archive.tar.gz"
    if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "$staging/archive.tar.gz" | awk '{print $1}')
    else actual=$(shasum -a 256 "$staging/archive.tar.gz" | awk '{print $1}')
    fi
    [ "$actual" = "$expected" ] || { printf 'checksum mismatch for %s\n' "$tool" >&2; exit 1; }
    tar -xzOf "$staging/archive.tar.gz" "$member" > "$staging/$tool"
    chmod 755 "$staging/$tool"
    mv "$staging/$tool" "$destination/$tool"
    printf 'installed CI tool %s %s\n' "$tool" "$version"
done
