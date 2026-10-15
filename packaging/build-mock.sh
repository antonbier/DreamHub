#!/bin/sh
# Builds clickable demo packages (mock mode, no Dreambox needed) for a PC.
# Usage: sh packaging/build-mock.sh [outdir]   (run `npm run build` in frontend/ first)
set -eu
OUT="${1:-dist}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"
if ! ls "$ROOT"/backend/internal/webui/dist/assets/*.js >/dev/null 2>&1; then
	echo "no built UI in backend/internal/webui/dist - run 'npm run build' in frontend/ first" >&2
	exit 1
fi
for T in linux/amd64 windows/amd64 darwin/arm64 darwin/amd64; do
	GOOS="${T%/*}"; GOARCH="${T#*/}"
	NAME="dreamhub-demo-$GOOS-$GOARCH"
	STAGE="$(mktemp -d)/$NAME"
	mkdir -p "$STAGE"
	EXT=""; [ "$GOOS" = windows ] && EXT=".exe"
	(cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -o "$STAGE/dreamhub$EXT" ./cmd/dreamhub)
	if [ "$GOOS" = windows ]; then
		printf '@echo off\r\nset DREAMHUB_MODE=mock\r\nset DREAMHUB_LISTEN_ADDR=127.0.0.1:8080\r\nset DREAMHUB_DATA_DIR=%%~dp0data\r\necho DreamHub demo: open http://localhost:8080 in your browser (Ctrl+C to stop)\r\nstart http://localhost:8080\r\ndreamhub.exe\r\n' >"$STAGE/start.bat"
	else
		printf '#!/bin/sh\ncd "$(dirname "$0")"\nexport DREAMHUB_MODE=mock DREAMHUB_LISTEN_ADDR=127.0.0.1:8080 DREAMHUB_DATA_DIR="$PWD/data"\necho "DreamHub demo: open http://localhost:8080 in your browser (Ctrl+C to stop)"\nchmod +x ./dreamhub\nexec ./dreamhub\n' >"$STAGE/start.sh"
		chmod +x "$STAGE/start.sh"
	fi
	printf 'DreamHub demo (mock data, no Dreambox needed).\nStart: %s\nThen open http://localhost:8080\nThe demo clock is fixed to the evening of 2026-09-23; recording plans in the past are refused.\nDelete the data folder to start fresh.\n' "$( [ "$GOOS" = windows ] && echo start.bat || echo "sh start.sh" )" >"$STAGE/README.txt"
	(cd "$(dirname "$STAGE")" && rm -f "$OUT/$NAME.zip" && python3 -c "import shutil,sys; shutil.make_archive(sys.argv[1], 'zip', '.', sys.argv[2])" "$OUT/$NAME" "$NAME")
	echo "built $OUT/$NAME.zip"
done
