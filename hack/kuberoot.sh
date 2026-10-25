#!/usr/bin/env bash
# Builds kuberoot as the distribution «1Ж:Оркестратор 8»: the sources of
# kuberoot with distros/assnetes added (the profile of edge under another
# name, and the add-ons of distro/kuberoot/assnetes, written in the 1C
# language and translated here), driven by kuberoot's own hack/hetzner.sh
# in a builder of the tenant assnetes.
#
#   hack/kuberoot.sh build amd64
#   hack/kuberoot.sh media amd64 [kernel args]
#   hack/kuberoot.sh node <name> amd64
#   hack/kuberoot.sh console <name>
#
#   KUBEROOT_SRC  a checkout of kuberoot (default: ../kuberoot)
#   KUBEROOT_REF  what of it to build (default: HEAD)
#   NS            the builder's namespace (default: tenant-assnetes)
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
SRC=${KUBEROOT_SRC:-$ROOT/../kuberoot}
REF=${KUBEROOT_REF:-HEAD}
NS=${NS:-tenant-assnetes}

work=$(mktemp -d)
trap 'rm -rf "${work}"' EXIT

git -C "${SRC}" archive "${REF}" | tar -x -C "${work}"
# hetzner.sh ships what git sees
git -C "${work}" init -q

d="${work}/distros/assnetes"
cp -R "${work}/distros/edge" "${d}"
sed -i.bak -E 's/^  name: edge$/  name: assnetes/' "${d}/profile.yaml" && rm "${d}/profile.yaml.bak"
grep -q '^  name: assnetes$' "${d}/profile.yaml" || { echo "profile of edge changed: no metadata.name to replace" >&2; exit 1; }

(cd "${ROOT}" && go build -o "${work}/1sctl" ./cmd/1sctl)
for src in "${ROOT}"/distro/kuberoot/assnetes/addons/*.bsl; do
  "${work}/1sctl" провести --пробно --вывод api --файл "${src}" \
    --параметр "Ключ=$(cat "${ROOT}/keys/index.pub")" \
    > "${d}/addons/$(basename "${src}" .bsl).yaml"
done
rm "${work}/1sctl"

for f in "${work}"/hack/hetzner.sh "${work}"/hack/hetzner/*.yaml; do
  sed -i.bak "s/tenant-kuberoot/${NS}/g" "${f}" && rm "${f}.bak"
done

KUBEROOT_DISTRO=assnetes "${work}/hack/hetzner.sh" "$@"
