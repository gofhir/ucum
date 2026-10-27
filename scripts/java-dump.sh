#!/usr/bin/env bash
# Regenera la salida versionada de Ucum-java (plan docs/plans/002-compare-java.md):
# descarga el JAR y UcumCommonUnitsTest.java con SHA-256 fijado en
# scripts/java.sha256, regenera los casos (necesita los packages, como
# make check-packages), compila y corre tools/javadump dos veces (las dos
# salidas tienen que ser iguales byte a byte) y reescribe el resultado y el
# manifiesto en internal/engine/testdata/java/. Avisa si Maven Central tiene
# una versión más nueva que la fijada. Necesita curl, shasum, go y un JDK 17+.
set -euo pipefail
here=$(cd "$(dirname "$0")/.." && pwd)
version=1.0.10
commit=db9d22ade3f0f4a667b45817b85a9ac4534e4224
cache=$here/.cache/java
data=$here/internal/engine/testdata/java
mkdir -p "$cache" "$data"

fetch() { # archivo url
  local file=$1 url=$2 sum
  sum=$(awk -v f="$file" '$2 == f {print $1}' "$here/scripts/java.sha256")
  [[ -n $sum ]] || { echo "$file: no SHA-256 in scripts/java.sha256"; exit 1; }
  if [[ -f $cache/$file ]] && ! (cd "$cache" && echo "$sum  $file" | shasum -a 256 -c --status); then
    echo "$file does not match its SHA-256: downloading again"
    rm -f "$cache/$file"
  fi
  if [[ ! -f $cache/$file ]]; then
    echo "downloading $url"
    curl -sSfL --proto '=https' --max-time 120 -o "$cache/$file.part" "$url"
    if ! (cd "$cache" && echo "$sum  $file.part" | shasum -a 256 -c --status); then
      rm -f "$cache/$file.part"
      echo "$file: download does not match its SHA-256"
      exit 1
    fi
    mv "$cache/$file.part" "$cache/$file"
  fi
}

fetch "ucum-$version.jar" "https://repo1.maven.org/maven2/org/fhir/ucum/$version/ucum-$version.jar"
fetch UcumCommonUnitsTest.java \
  "https://raw.githubusercontent.com/FHIR/Ucum-java/$commit/src/test/java/org/fhir/ucum/UcumCommonUnitsTest.java"
"$here/scripts/fetch-packages.sh"

# Solo un aviso: sin red, la corrida sigue con el JAR del cache.
latest=$(curl -sSf --proto '=https' --max-time 60 https://repo1.maven.org/maven2/org/fhir/ucum/maven-metadata.xml |
  sed -n 's:.*<release>\(.*\)</release>.*:\1:p') || latest=unknown
if [[ $latest != "$version" ]]; then
  echo "WARNING: Maven Central has org.fhir:ucum $latest; this comparison is pinned to $version"
fi

(cd "$here/internal/engine" &&
  UCUM_JAVA_CASES=$data/cases.tsv UCUM_PACKAGES=$here/.cache/packages \
  UCUM_JAVA_COMMON_SRC=$cache/UcumCommonUnitsTest.java \
  go test -count=1 -run '^TestWriteJavaCases$' .)

rm -rf "$cache/classes"
javac -encoding UTF-8 -d "$cache/classes" -cp "$cache/ucum-$version.jar" "$here/tools/javadump/JavaDump.java"
cp="$cache/ucum-$version.jar:$cache/classes"
out=$data/ucum-java-$version.tsv
java -cp "$cp" JavaDump < "$data/cases.tsv" > "$out.part"
java -cp "$cp" JavaDump < "$data/cases.tsv" > "$cache/second.tsv"
cmp -s "$out.part" "$cache/second.tsv" || { echo "two runs of the dumper differ"; exit 1; }
mv "$out.part" "$out"

sha() { shasum -a 256 "$1" | cut -d' ' -f1; }
cat > "$data/manifest.json" <<JSON
{
  "reference": "org.fhir:ucum:$version",
  "commit": "$commit",
  "jar_sha256": "$(sha "$cache/ucum-$version.jar")",
  "common_units_test_sha256": "$(sha "$cache/UcumCommonUnitsTest.java")",
  "dumper_sha256": "$(sha "$here/tools/javadump/JavaDump.java")",
  "cases_sha256": "$(sha "$data/cases.tsv")",
  "common_units_sha256": "$(sha "$data/common-units.tsv")",
  "result_sha256": "$(sha "$out")"
}
JSON
cat > "$data/run-info.json" <<JSON
{
  "date": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "java_version": "$(java -version 2>&1 | head -1 | sed 's/"/\\"/g')",
  "maven_latest": "$latest"
}
JSON
echo "$(wc -l < "$out" | tr -d ' ') results in $out"
