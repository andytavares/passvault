#!/bin/sh
# Fails when any Go module in the build graph was published less than
# MIN_AGE_DAYS ago. Go has no built-in minimum-release-age setting.
set -eu

MIN_AGE_DAYS="${MIN_AGE_DAYS:-14}"
cutoff=$(date -u -v-"${MIN_AGE_DAYS}"d +%Y-%m-%dT%H:%M:%SZ 2>/dev/null ||
	date -u -d "-${MIN_AGE_DAYS} days" +%Y-%m-%dT%H:%M:%SZ)

too_new=$(go list -m -f '{{if not .Main}}{{.Time.UTC.Format "2006-01-02T15:04:05Z"}} {{.Path}}@{{.Version}}{{end}}' all |
	awk -v cutoff="$cutoff" 'NF == 2 && $1 > cutoff { print }')

if [ -n "$too_new" ]; then
	echo "Modules published after $cutoff (younger than $MIN_AGE_DAYS days):" >&2
	echo "$too_new" >&2
	exit 1
fi
echo "check-dep-age: all modules older than $MIN_AGE_DAYS days (cutoff $cutoff)"
