#!/usr/bin/env bash
# Descarga los packages FHIR oficiales listados en scripts/packages.sha256
# (los ejemplos oficiales de R4, R4B y R5), o en la lista que se pase como
# argumento, a .cache/packages/ y verifica sus SHA-256. La URL de descarga es
# la que publica el registro packages.fhir.org en la metadata del package
# (dist.tarball), como hace npm. Necesita curl, shasum y python3.
set -euo pipefail
here=$(cd "$(dirname "$0")/.." && pwd)
dir=$here/.cache/packages
list=${1:-$here/scripts/packages.sha256}
# Absoluta: la verificación final corre dentro de $dir.
list=$(cd "$(dirname "$list")" && pwd)/$(basename "$list")
mkdir -p "$dir"
# El || recoge una última línea sin salto de línea final.
while read -r sum file || [[ -n $file ]]; do
  [[ -z $file ]] && continue
  name=${file%-*}; version=${file##*-}; version=${version%.tgz}
  # Un archivo que no coincide con su SHA-256 (descarga cortada, corrupto) se
  # vuelve a descargar.
  if [[ -f $dir/$file ]] && ! (cd "$dir" && echo "$sum  $file" | shasum -a 256 -c --status); then
    echo "$file does not match its SHA-256: downloading again"
    rm -f "$dir/$file"
  fi
  if [[ ! -f $dir/$file ]]; then
    url=$(curl -sSf --max-time 60 "https://packages.fhir.org/$name" |
      python3 -I -c 'import json,sys; print(json.load(sys.stdin)["versions"][sys.argv[1]]["dist"]["tarball"])' "$version")
    # Solo HTTPS; y un archivo que no coincide con su SHA-256 no se queda.
    [[ $url == https://* ]] || { echo "$name $version: tarball URL is not HTTPS: $url"; exit 1; }
    echo "downloading $name $version ($url)"
    curl -sSfL --proto '=https' --max-time 600 -o "$dir/$file.part" "$url"
    if ! (cd "$dir" && echo "$sum  $file.part" | shasum -a 256 -c --status); then
      rm -f "$dir/$file.part"
      echo "$file: download does not match its SHA-256"
      exit 1
    fi
    mv "$dir/$file.part" "$dir/$file"
  fi
done < "$list"
(cd "$dir" && shasum -a 256 -c "$list")
