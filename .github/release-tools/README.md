# Release matrix

The release workflow builds the frontend once, then runs each configured Go target on its own Linux runner. `CGO_ENABLED=0` permits cross-compilation. The target matrix is read from `.goreleaser.yaml`, including its exclusions; simple releases select Linux amd64 only.

Each build uses GoReleaser OSS in snapshot mode with the selected release version and one target. Archive naming, bundled files, Go flags and release templates remain in the existing GoReleaser configurations. Every archive is accompanied by its source commit, target, version and SHA256. The publishing job verifies the complete matrix before building images or publishing. It uses GoReleaser's `extra_files` support to publish existing archives and checksums, with builds disabled. No Pro license is needed.

Go caches are isolated by target and refreshed on each source commit, with fallback to the preceding target cache. Save uses the original restore key, even if a build hook changes `go.sum`. Matrix jobs upload uniquely named artifacts. The publishing job extracts only the regular Linux binary from each verified archive and restores its executable permission before constructing Docker contexts. QEMU remains limited to runtime-image instructions. DockerHub images are omitted when its credentials are absent; GHCR is always retained. Simple mode still publishes only the amd64 GHCR image and the simple release description.

All build jobs use the commit resolved by `prepare`, including a manual release's selected tag. Helper scripts come from the workflow revision and are passed as a run-local artifact, so older application tags do not need to contain the new scripts. The workflow serializes release runs to prevent simultaneous updates to moving image tags.

## Validate without publication

From a branch containing this workflow:

```bash
gh workflow run release.yml --ref <branch> \
  -f tag=<branch> -f dry_run=true -f simple_release=false
```

A dry run builds all selected archives and both runtime images, verifies artifact provenance and produces the final checksum file. It exports images locally as OCI archives instead of pushing them. It skips registry logins, GitHub Release publication, DockerHub description updates, Telegram notifications and VERSION synchronization. Test the simple path separately with `simple_release=true`.

Dry-run artifacts are available in the Actions run, including `release-dry-run-report`. Compare job start/end times, GoReleaser's build duration and cache restore results. Do not present an initial cold-cache run as a warmed-cache benchmark; publishing network time is not measured by dry runs.

Helper checks:

```bash
python -m pip install -r .github/release-tools/requirements-release.txt
python -m unittest discover -s .github/release-tools -p 'test_release_matrix.py'
bash -n .github/release-tools/release-images.sh
```

## Release notes

`generate_release_notes.py` builds the GitHub Release body from git history, so every release lists what actually changed instead of an empty section.

```bash
# 预览最近一个 tag 的发布说明
python .github/release-tools/generate_release_notes.py --tag "$(git describe --tags --abbrev=0)"
# 预览并指定对比基线
python .github/release-tools/generate_release_notes.py --tag v0.2.68 --previous-tag v0.2.67
# 等价写法
make changelog TAG=v0.2.68 CHANGELOG_BASE=v0.2.67
```

Behaviour:

- Commits are grouped by conventional-commit type: `feat` / `fix` / `perf` / `refactor` / `revert` / `docs`, with `build|ci|chore|style|test` folded into 维护 and anything unrecognised into 其他变更.
- Version-bump and release-plumbing commits are filtered out.
- Merges of `upstream/main` collapse into a single 上游同步 entry with the commit count, so a fork rebase does not dump hundreds of foreign commits into the notes.
- An annotated tag body, if present, is kept above the generated sections for hand-written highlights.
- `--max-items N` caps each section (default 30), `--no-tag-body` drops the tag body, `--previous-tag` overrides the baseline.

The release workflow runs it in the `release` job and passes the result to GoReleaser as `TAG_MESSAGE`, which feeds both the Release body and the Telegram notification.
