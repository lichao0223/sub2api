#!/usr/bin/env bash
set -euo pipefail
: "${RELEASE_VERSION:?}" "${RELEASE_SHA:?}" "${GITHUB_REPOSITORY:?}" "${RUNNER_TEMP:?}"
owner=${GITHUB_REPOSITORY%%/*}
registries=("ghcr.io/${owner,,}/sub2api")
if [[ ${SIMPLE_RELEASE:-false} != true && ${DOCKERHUB_USERNAME:-skip} != skip ]]; then
  registries+=("${DOCKERHUB_USERNAME}/sub2api")
fi
mapfile -t arches < <(find .release-context -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | sort)
if ((${#arches[@]} == 0)); then
  echo 'no release contexts found' >&2
  exit 1
fi
for arch in "${arches[@]}"; do
  args=(--platform "linux/$arch" --file ".release-context/$arch/Dockerfile"
    --label "org.opencontainers.image.version=$RELEASE_VERSION"
    --label "org.opencontainers.image.revision=$RELEASE_SHA"
    --label "org.opencontainers.image.source=https://github.com/$GITHUB_REPOSITORY")
  for registry in "${registries[@]}"; do
    args+=(--tag "$registry:$RELEASE_VERSION-$arch")
    if [[ ${SIMPLE_RELEASE:-false} == true ]]; then
      args+=(--tag "$registry:$RELEASE_VERSION" --tag "$registry:latest")
    fi
  done
  if [[ ${DRY_RUN:-false} == true ]]; then
    args+=(--output "type=oci,dest=$RUNNER_TEMP/sub2api-$arch.oci.tar")
  else
    args+=(--push)
  fi
  docker buildx build "${args[@]}" ".release-context/$arch"
done
if [[ ${DRY_RUN:-false} != true && ${#arches[@]} -gt 1 ]]; then
  major=${RELEASE_VERSION%%.*}
  minor=${RELEASE_VERSION#*.}; minor=${minor%%.*}
  for registry in "${registries[@]}"; do
    sources=()
    for arch in "${arches[@]}"; do
      sources+=("$registry:$RELEASE_VERSION-$arch")
    done
    docker buildx imagetools create \
      --tag "$registry:$RELEASE_VERSION" --tag "$registry:latest" \
      --tag "$registry:$major.$minor" --tag "$registry:$major" \
      "${sources[@]}"
  done
fi
