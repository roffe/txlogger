#!/usr/bin/env bash
# Shows which of txlogger's dependencies are behind.
set -euo pipefail
cd "$(dirname "$0")/.."
export GOWORK=off

declare -A branch=([roffe/fyne]=wgpu)

echo "== own modules (pinned vs branch tip on GitHub)"
go list -m -f '{{.Path}} {{.Version}}{{with .Replace}} {{.Path}} {{.Version}}{{end}}' all |
while read -r path ver rpath rver; do
	if [[ -n ${rpath:-} ]]; then path=$rpath ver=$rver; fi
	[[ $path == github.com/roffe/* && $path != github.com/roffe/txlogger ]] || continue
	repo=${path#github.com/}
	repo=${repo%/v[0-9]*}
	url=https://github.com/$repo.git
	ref=${branch[$repo]:+refs/heads/${branch[$repo]}}
	tip=$(git ls-remote "$url" "${ref:-HEAD}" | cut -c1-12)
	if [[ $ver =~ -([0-9a-f]{12})$ ]]; then
		pin=${BASH_REMATCH[1]}
	else
		pin=$(git ls-remote "$url" "refs/tags/$ver" "refs/tags/$ver^{}" | tail -1 | cut -c1-12)
	fi
	if [[ $pin == "$tip" ]]; then state=ok; else state="BEHIND, tip $tip"; fi
	printf '%-28s %-44s %s\n' "$repo" "$ver" "$state"
done

fyne() { grep -o 'roffe/fyne/v2 v[^ ]*' "$1" | cut -d' ' -f2; }
if [[ -f ../bdmtool/go.mod && $(fyne go.mod) != "$(fyne ../bdmtool/go.mod)" ]]; then
	echo "!! bdmtool pins fyne $(fyne ../bdmtool/go.mod), go.work builds will fail until it matches"
fi

echo "== third-party direct deps with updates"
go list -m -u -f '{{if and .Update (not .Indirect) (not .Replace)}}{{.Path}} {{.Version}} -> {{.Update.Version}}{{end}}' all | grep -v '^$' || echo "none"
