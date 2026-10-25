#!/usr/bin/env bash
# Writes the manifests of the packages of operators from deploy/obrabotchik.bsl:
# the packages are built from manifests in the 1C language, through 1сctl.
#
#   IMAGE   the image of obrabotchik (default: ghcr.io/tym83/assnetes-obrabotchik:main)
set -euo pipefail

cd "$(dirname "$0")/.."
IMAGE=${IMAGE:-ghcr.io/tym83/assnetes-obrabotchik:main}
CTL=$(mktemp -d)/1sctl
trap 'rm -rf "$(dirname "${CTL}")"' EXIT
go build -o "${CTL}" ./cmd/1sctl

# package          name of obrabotchik   namespace           operators
while read -r package name namespace operators; do
  mkdir -p "recipes/${package}/manifests"
  "${CTL}" провести --пробно --вывод api --файл deploy/obrabotchik.bsl \
    --параметр "Имя=${name}" --параметр "Подразделение=${namespace}" \
    --параметр "Обработки=${operators}" --параметр "Образ=${IMAGE}" \
    --параметр СоздатьПодразделение=Нет \
    > "recipes/${package}/manifests/obrabotchik.yaml"
  echo "${package}: ${operators}"
done <<'LIST'
assnetes-obrabotki obrabotchik assnetes-system proizvodstvennyj-kalendar,kursy-valyut,data-zapreta,udalenie-pomechennyh,akt-sverki
1c-bitrix obrabotchik-bitrix assnetes-bitrix bitrix
LIST
