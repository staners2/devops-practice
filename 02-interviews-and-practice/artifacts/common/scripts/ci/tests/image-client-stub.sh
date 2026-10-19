#!/bin/sh
set -eu
printf '%s\n' "$0 $*" >> "$IMAGE_TEST_LOG"
case "${0##*/}" in
  buildctl-daemonless.sh) ;;
  skopeo)
    case "$1" in
      login) IFS= read -r password || [ -n "$password" ]; [ "$password" = "$CI_REGISTRY_PASSWORD" ] ;;
      copy)
        [ "${IMAGE_TEST_FAIL_COPY:-0}" = 0 ] || exit 1
        for argument do
          case "$argument" in docker-archive:*) printf 'single-image fixture\n' > "${argument#docker-archive:}" ;; esac
        done ;;
      inspect)
        if [ "${3:-}" = '{{.Architecture}}' ]; then
          case "${4:-}" in *amd64.tar) architecture=amd64 ;; *arm64.tar) architecture=arm64 ;; *) exit 1 ;; esac
          printf '%s\n' "${IMAGE_TEST_ARCH:-$architecture}"
        else
          printf '%s\n' "${IMAGE_TEST_DIGEST:-sha256:1111111111111111111111111111111111111111111111111111111111111111}"
        fi ;;
      *) exit 1 ;;
    esac ;;
  *) exit 1 ;;
esac
