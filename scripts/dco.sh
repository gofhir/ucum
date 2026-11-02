#!/bin/sh
# Verifica el Developer Certificate of Origin (CONTRIBUTING.md) en los commits
# de base..head: cada uno lleva un Signed-off-by con el correo de su autor.
# Quedan fuera el titular del copyright (LICENSE), cuya obra ya es suya, y los
# bots de GitHub (release-please, dependabot), que no aportan obra propia.
#
#   scripts/dco.sh <base> <head>
set -eu

base=$1
head=$2

# El correo del titular del copyright de LICENSE.
holder="robaraneda@gmail.com"

# Una asignación aparte: un rango inválido detiene el script (set -e), en vez de
# dar un for vacío que aprueba sin mirar nada.
commits=$(git rev-list --no-merges "$base..$head")

status=0
for c in $commits; do
	email=$(git log -1 --format='%ae' "$c")
	case $email in
	"$holder" | *"[bot]@users.noreply.github.com") continue ;;
	esac
	if ! git log -1 --format='%B' "$c" | grep -i '^Signed-off-by: ' | grep -qiF "<$email>"; then
		echo "commit $(git log -1 --format='%h %s' "$c"): no Signed-off-by for its author <$email>; see CONTRIBUTING.md"
		status=1
	fi
done
exit $status
