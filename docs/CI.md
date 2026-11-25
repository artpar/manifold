# Automated CI and releases

[Workflow](../.github/workflows/ci.yml) · [Hosted runs](https://github.com/artpar/manifold/actions/workflows/ci.yml) · [Releases](https://github.com/artpar/manifold/releases)

## Continuous integration

Pushes to `main`, pull requests and version tags run checks against the module's
Go version. Linux/macOS/Windows run module verification, vet, tests and an actual
CGO-disabled CLI build/help execution, including presentation command help. Linux also checks gofmt/version syntax and
runs the complete race suite with a 20-minute package timeout. Pull requests have
read-only permissions and cannot publish. Release jobs depend on all checks.

The ordinary CI path uses Go and shell; optional Python/JavaScript qualification
scripts are not required for building, running or testing the production library.
Historical independent-reader/visual evidence remains in the qualification docs.

## Continuous delivery

`VERSION` contains a stable semantic version without `v` (initially `0.1.0`).
Merge a reviewed version bump into `main`. After CI passes, Actions:

1. Determines whether that version has already been published.
2. Builds CGO-disabled CLI binaries for Linux, macOS and Windows on amd64/arm64.
3. Packages each binary with LICENSE, THIRD_PARTY.md, dependency licenses and
   version/source-commit information.
4. Generates and verifies SHA-256 checksums for all six archives.
5. Creates `v<VERSION>` pointing at the checked commit, creates/uploads a draft
   GitHub release, then publishes it only after packaging/upload succeeds.

Existing published versions are not rebuilt on later commits. Failed draft
publication can be retried by rerunning its Actions job. Version tags must match
VERSION and are never force-moved. Manually pushed matching tags also run the
same gated workflow, but no manual tag/build/upload is required for the normal
version-bump path. Tags created by the workflow do not need another workflow run;
the same job completes publication.

Only the release job receives repository write permission through GITHUB_TOKEN;
there is no personal release token or additional service configuration. Actions
are pinned to exact upstream commits. Archives contain CLI artifacts/licenses,
not test documents, ignored QA environments or customer files.

## Verification and recovery

Use hosted check logs and the release's checksum manifest as the current evidence.
A workflow file alone does not establish a successful run. The CLI's Linux release
binary is smoke-tested before upload; the check matrix also executes a native
CGO-disabled CLI on all three operating systems. Cross-built arm64/amd64 variants
are not all executed individually.

If checks fail, fix the source/workflow and push normally. If the version tag was
created but upload failed, rerun the failed job at that same commit. Do not replace
an already published version; bump VERSION for subsequent source changes.

## Latest verified publication

v0.4.1 was published automatically on 2026-11-25 at 17:21:02 UTC from
`bdb8ed3ee8af40256e2eb3851032800917464784`.
[Release](https://github.com/artpar/manifold/releases/tag/v0.4.1) ·
[Successful gated run](https://github.com/artpar/manifold/actions/runs/45660838689).
Windows/macOS/Linux checks and the full Linux race gate passed; the pipeline
built six CGO-disabled platform archives and checksums.txt. After publication,
the tag target and latest-release status were verified, and all six manifest
hashes matched GitHub's SHA-256 asset digests. No workstation build/upload or
manual tag was used. Later KT-only commits do not move this release tag.
