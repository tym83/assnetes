#!/usr/bin/env bash
# Writes the manifests of the packages from sources in the 1C language,
# through 1сctl: the runner of each package of operators from
# deploy/obrabotchik.bsl, the kinds of its operators from their
# descriptions (1сctl конфигуратор), and the objects a package brings from
# the .bsl files next to its recipe.
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
  out="recipes/${package}/manifests"
  mkdir -p "${out}"
  "${CTL}" провести --пробно --вывод api --файл deploy/obrabotchik.bsl \
    --параметр "Имя=${name}" --параметр "Подразделение=${namespace}" \
    --параметр "Обработки=${operators}" --параметр "Образ=${IMAGE}" \
    --параметр СоздатьПодразделение=Нет --параметр ВидыСтавитПакет=Да \
    > "${out}/obrabotchik.yaml"
  : > "${out}/vidy.yaml"
  for o in ${operators//,/ }; do
    if compgen -G "obrabotki/${o}/Метаданные*.yaml" > /dev/null; then
      [[ -s "${out}/vidy.yaml" ]] && echo "---" >> "${out}/vidy.yaml"
      "${CTL}" конфигуратор "obrabotki/${o}" >> "${out}/vidy.yaml"
    fi
  done
  [[ -s "${out}/vidy.yaml" ]] || rm "${out}/vidy.yaml"
  echo "${package}: ${operators}"
done <<'LIST'
assnetes-obrabotki obrabotchik assnetes-system proizvodstvennyj-kalendar,kursy-valyut,data-zapreta,udalenie-pomechennyh,akt-sverki,glavnaya-kniga
1c-bitrix obrabotchik-bitrix assnetes-bitrix bitrix
1c-predpriyatie obrabotchik-1c assnetes-1c 1c-predpriyatie
LIST

# Packages of objects: whatever their .bsl files post.
for src in recipes/*/*.bsl; do
  out="$(dirname "${src}")/manifests"
  mkdir -p "${out}"
  "${CTL}" провести --пробно --вывод api --файл "${src}" > "${out}/$(basename "${src}" .bsl).yaml"
  echo "$(dirname "${src}" | xargs basename): ${src}"
done
