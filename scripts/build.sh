#!/bin/sh
set -eu

project_dir=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
cd "$project_dir"
output_dir="$project_dir/build/bin"

fail() { printf '%s\n' "$*" >&2; exit 1; }
require() { command -v "$1" >/dev/null 2>&1 || fail "Ferramenta ausente: $1. Consulte os requisitos de build no README."; }
require go
: "${APP_VERSION:=0.1.0}"
: "${APP_ARCH:=$(go env GOHOSTARCH)}"
: "${WAILS_BIN:=$project_dir/build/tools/wails-v2.15.0/wails}"
: "${NFPM_BIN:=$project_dir/build/tools/nfpm-v2.48.0/nfpm}"
case "$APP_ARCH" in amd64|arm64) ;; *) fail 'ARCH inválida. Use amd64 ou arm64.' ;; esac
case "$APP_VERSION" in ''|*[!0-9A-Za-z.+-]*) fail 'VERSION inválida. Use uma versão como 0.1.0.' ;; esac

check_host() {
    [ "$(go env GOHOSTOS)" = "$1" ] || fail "Este target exige $1. Execute o build nesse sistema operacional."
    [ "$APP_ARCH" = "$(go env GOHOSTARCH)" ] || fail 'Este target exige ARCH igual à arquitetura nativa da máquina.'
}
check_linux() {
    check_host linux
    require pkg-config
    pkg-config --exists gtk+-3.0 webkit2gtk-4.1 odbc || fail 'Instale as dependências GTK3, WebKitGTK 4.1 e unixODBC indicadas no README.'
}
check_mac() {
    check_host darwin
    require xcode-select
    xcode-select -p >/dev/null 2>&1 || fail 'Instale Xcode Command Line Tools com xcode-select --install.'
    require mvn
    require java
    require hdiutil
}

case "${1:-}" in
    check-linux) check_linux ;;
    check-mac) check_mac ;;
    linux)
        check_linux
        require npm
        mkdir -p "$output_dir"
        "$WAILS_BIN" build -m -nosyncgomod -platform "linux/$APP_ARCH" -tags 'odbc,webkit2_41' -o "go-db-viewer-linux-$APP_ARCH"
        ;;
    exe)
        require npm
        mkdir -p "$output_dir"
        CGO_ENABLED=0 "$WAILS_BIN" build -m -nosyncgomod -platform "windows/$APP_ARCH" -o "go-db-viewer-windows-$APP_ARCH.exe"
        ;;
    deb|rpm)
        check_linux
        require readelf
        APP_BINARY="$output_dir/go-db-viewer-linux-$APP_ARCH"
        [ -x "$APP_BINARY" ] || fail 'Executável Linux ausente. Execute make build-linux.'
        GLIBC_VERSION=$(LC_ALL=C readelf --version-info "$APP_BINARY" | sed -n 's/.*Name: GLIBC_\([0-9][0-9.]*\).*/\1/p' | sort -Vu | tail -n 1)
        [ -n "$GLIBC_VERSION" ] || fail 'Os pacotes .deb e .rpm exigem um executável Linux com glibc.'
        if [ -z "${MAINTAINER:-}" ]; then
            MAINTAINER=$(git log -1 --format='%an <%ae>' 2>/dev/null || printf '%s' 'Go DB Viewer')
        fi
        export APP_VERSION APP_ARCH APP_BINARY GLIBC_VERSION MAINTAINER
        "$NFPM_BIN" package --config packaging/nfpm.yaml --packager "$1" --target "$output_dir/"
        ;;
    mac)
        check_mac
        require npm
        mkdir -p "$output_dir"
        mvn -B -f ignite-bridge/pom.xml package
        "$WAILS_BIN" build -m -nosyncgomod -platform "darwin/$APP_ARCH" -o go-db-viewer
        app="$output_dir/go-db-viewer.app"
        [ -d "$app/Contents/Resources" ] || fail 'O Wails não gerou o bundle .app esperado.'
        for jar in ignite-bridge.jar ignite2-jdbc.jar ignite3-jdbc.jar; do
            [ -f "ignite-bridge/target/$jar" ] || fail "JAR ausente: $jar. Verifique o build Maven."
            cp "ignite-bridge/target/$jar" "$app/Contents/Resources/$jar"
        done
        staging=$(mktemp -d "$project_dir/build/dmg.XXXXXX")
        trap 'rm -rf "$staging"' EXIT HUP INT TERM
        cp -R "$app" "$staging/go-db-viewer.app"
        ln -s /Applications "$staging/Applications"
        hdiutil create -volname 'Go DB Viewer' -srcfolder "$staging" -format UDZO -ov "$output_dir/go-db-viewer-darwin-$APP_ARCH.dmg"
        ;;
    *) fail 'Target inválido. Execute make help.' ;;
esac
