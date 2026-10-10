#!/bin/sh
set -eu

for command in curl git jq skopeo; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "$command is required" >&2
    exit 1
  fi
done

for variable in \
  CI_API_V4_URL CI_PROJECT_ID CI_PROJECT_URL CI_REPOSITORY_URL \
  CI_REGISTRY CI_REGISTRY_IMAGE CI_REGISTRY_USER CI_REGISTRY_PASSWORD \
  CI_COMMIT_SHA CI_DEFAULT_BRANCH CI_JOB_TOKEN IMAGE_REFERENCE IMAGE_DIGEST; do
  eval "value=\${$variable:-}"
  if [ -z "$value" ]; then
    echo "$variable is required" >&2
    exit 1
  fi
done

if [ "$CI_DEFAULT_BRANCH" != main ]; then
  echo "CI_DEFAULT_BRANCH must be main for the tracked Argo CD targetRevision" >&2
  exit 1
fi
if ! printf '%s\n' "$CI_COMMIT_SHA" | grep -Eq '^[0-9a-f]{40}$'; then
  echo "CI_COMMIT_SHA must contain 40 lowercase hexadecimal characters" >&2
  exit 1
fi
if ! printf '%s\n' "$IMAGE_DIGEST" | grep -Eq '^sha256:[0-9a-f]{64}$'; then
  echo "IMAGE_DIGEST is invalid" >&2
  exit 1
fi
if [ "$IMAGE_REFERENCE" != "$CI_REGISTRY_IMAGE:$CI_COMMIT_SHA" ]; then
  echo "IMAGE_REFERENCE does not match the published commit image" >&2
  exit 1
fi
if ! printf '%s\n' "$CI_REGISTRY_IMAGE" \
  | grep -Eq '^[A-Za-z0-9._:-]+(/[A-Za-z0-9._-]+)+$'; then
  echo "CI_REGISTRY_IMAGE contains unsupported characters" >&2
  exit 1
fi

temp_dir=$(mktemp -d)
trap 'rm -rf "$temp_dir"' EXIT HUP INT TERM
auth_file=$temp_dir/auth.json
printf '%s' "$CI_REGISTRY_PASSWORD" | skopeo login \
  --authfile "$auth_file" \
  --username "$CI_REGISTRY_USER" \
  --password-stdin "$CI_REGISTRY" >/dev/null
actual_digest=$(skopeo inspect \
  --authfile "$auth_file" \
  --format '{{.Digest}}' \
  "docker://$IMAGE_REFERENCE")
if [ "$actual_digest" != "$IMAGE_DIGEST" ]; then
  echo "published image digest does not match image.env" >&2
  exit 1
fi

values_path=deploy/environments/development/values.yaml
promotion_branch=gitops/development-$CI_COMMIT_SHA
default_ref=refs/remotes/gitops/$CI_DEFAULT_BRANCH
promotion_ref=refs/remotes/gitops/promotion

image_repository() {
  awk '
    $0 == "image:" {in_image=1; next}
    in_image && /^[^[:space:]]/ {in_image=0}
    in_image && $1 == "repository:" {print $2; found++}
    END {if (found != 1) exit 1}
  ' "$1"
}

image_tag() {
  awk '
    $0 == "image:" {in_image=1; next}
    in_image && /^[^[:space:]]/ {in_image=0}
    in_image && $1 == "tag:" {print $2; found++}
    END {if (found != 1) exit 1}
  ' "$1"
}

has_expected_image() {
  [ "$(image_repository "$1")" = "$CI_REGISTRY_IMAGE" ] \
    && [ "$(image_tag "$1")" = "$CI_COMMIT_SHA" ]
}

trigger_validation_pipeline() {
  curl --fail --silent --show-error --request POST \
    --header "JOB-TOKEN: $CI_JOB_TOKEN" \
    --form "ref=$promotion_branch" \
    "$CI_API_V4_URL/projects/$CI_PROJECT_ID/trigger/pipeline" >/dev/null
  echo "GitOps validation pipeline triggered for $promotion_branch"
}

git fetch --quiet "$CI_REPOSITORY_URL" \
  "refs/heads/$CI_DEFAULT_BRANCH:$default_ref"
git show "$default_ref:$values_path" > "$temp_dir/default-values.yaml"
if has_expected_image "$temp_dir/default-values.yaml"; then
  echo "development already references the published image"
  exit 0
fi

mr_json=$(curl --fail --silent --show-error \
  --header "JOB-TOKEN: $CI_JOB_TOKEN" \
  --get \
  --data-urlencode state=opened \
  --data-urlencode "source_branch=$promotion_branch" \
  "$CI_API_V4_URL/projects/$CI_PROJECT_ID/merge_requests")
if ! printf '%s' "$mr_json" | jq -e 'type == "array"' >/dev/null; then
  echo "GitLab merge requests response is not an array" >&2
  exit 1
fi
mr_count=$(printf '%s' "$mr_json" | jq 'length')
if [ "$mr_count" -gt 1 ]; then
  echo "more than one open promotion merge request exists" >&2
  exit 1
fi

branch_exists=false
if git ls-remote --exit-code --heads "$CI_REPOSITORY_URL" \
  "refs/heads/$promotion_branch" >/dev/null 2>&1; then
  branch_exists=true
fi

if [ "$mr_count" -eq 1 ]; then
  if [ "$branch_exists" != true ]; then
    echo "open promotion merge request has no source branch" >&2
    exit 1
  fi
  git fetch --quiet "$CI_REPOSITORY_URL" \
    "+refs/heads/$promotion_branch:$promotion_ref"
  git show "$promotion_ref:$values_path" > "$temp_dir/promotion-values.yaml"
  if ! has_expected_image "$temp_dir/promotion-values.yaml"; then
    echo "open promotion merge request references a different image" >&2
    exit 1
  fi
  mr_url=$(printf '%s' "$mr_json" | jq -r '.[0].web_url')
  echo "reusing open promotion merge request: $mr_url"
  trigger_validation_pipeline
  exit 0
fi

if [ "$branch_exists" = true ]; then
  echo "promotion branch exists without an open merge request" >&2
  exit 1
fi

git switch --detach "$default_ref" >/dev/null
git switch -C "$promotion_branch" >/dev/null
updated_values=$temp_dir/values.yaml
if ! awk -v repository="$CI_REGISTRY_IMAGE" -v tag="$CI_COMMIT_SHA" '
  $0 == "image:" {in_image=1; print; next}
  in_image && /^[^[:space:]]/ {in_image=0}
  in_image && $1 == "repository:" {
    print "  repository: " repository
    repository_count++
    next
  }
  in_image && $1 == "tag:" {
    print "  tag: " tag
    tag_count++
    next
  }
  {print}
  END {
    if (repository_count != 1 || tag_count != 1) exit 42
  }
' "$values_path" > "$updated_values"; then
  echo "development values must contain one image repository and tag" >&2
  exit 1
fi
mv "$updated_values" "$values_path"

if ! has_expected_image "$values_path"; then
  echo "development values were not updated to the expected image" >&2
  exit 1
fi
git diff --check -- "$values_path"
if git diff --quiet -- "$values_path"; then
  echo "development already references the published image"
  exit 0
fi

git config user.name 'GitLab GitOps Promotion'
git config user.email 'gitops-promotion@noreply.gitlab.com'
git add "$values_path"
git commit -m "chore: promote $CI_COMMIT_SHA to development" >/dev/null
git push "$CI_REPOSITORY_URL" "HEAD:refs/heads/$promotion_branch" \
  -o merge_request.create \
  -o "merge_request.target=$CI_DEFAULT_BRANCH" \
  -o "merge_request.title=GitOps: deploy $CI_COMMIT_SHA to development" \
  -o merge_request.remove_source_branch >/dev/null

echo "created promotion merge request for $CI_COMMIT_SHA"
trigger_validation_pipeline
