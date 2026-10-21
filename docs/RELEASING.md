# Releasing

A release starts when a signed `v*` tag is pushed. The workflow `.github/workflows/release.yml` does the
rest. This page is the release manager's checklist, followed by the one-time setup.

## What the workflow does

1. `release` checks that the tag is a version and points at a commit on `main`. It builds `dummyrun` and
   `dummyrun-otel` with GoReleaser for Linux, macOS and Windows on amd64 and arm64. It writes the archives, the
   SBOMs and `checksums.txt`, signs the checksums with cosign and attests their build provenance. It creates a
   draft GitHub release with the notes from `CHANGELOG.md`.
2. `image` builds the image on the released Linux binaries and smoke tests it before pushing. It pushes
   `ghcr.io/dummy-run/dummyrun` for amd64 and arm64 by digest only. It signs the digest and attests its
   provenance and SBOM. Only then does it give the digest the version, minor version and `latest` tags.
3. `publish` takes the release out of draft. It runs only when both earlier jobs pass, so a failed run never
   leaves a public release without its image.

The jobs hold only the permissions they need. Only `release` reads the `release` environment, which holds the
Homebrew token.

## Checklist

1. Check that `main` is green: CI, Quality, Engine and Release check.
2. Check that `CHANGELOG.md` has a section for the version with the entries for it. Replace `unreleased` in its
   heading with the release date, as in `## [0.1.0] - 2026-10-21`. Open a pull request, get it merged and pull
   `main`. Leave `## [Unreleased]` above it, empty.
3. Check the pins. The browser versions in `docker/Dockerfile` must match `.github/workflows/engine.yml`, and the
   Release check workflow fails when they differ.
4. Rehearse the build on your machine if you changed `.goreleaser.yaml`. `scripts/goreleaser-check.sh` checks the
   configuration and `goreleaser build --snapshot --single-target --clean` builds one target.
5. Tag the merged commit with a signed tag and push only the tag.

   ```sh
   git switch main && git pull --ff-only
   git tag -s v0.1.0 -m "v0.1.0"
   git push origin v0.1.0
   ```

6. Watch the run with `gh run watch`. If the environment has a required reviewer, approve the `release` job
   when it waits.
7. Verify the artefacts with the commands under "Verify a download" and "Verify the image" in [INSTALL.md](INSTALL.md).
   - Download one archive and `checksums.txt`.
   - Run `cosign verify-blob` on the checksums file.
   - Run `sha256sum --ignore-missing -c` on the archive.
   - Run `gh attestation verify` on the archive.
   - Run `cosign verify` and `gh attestation verify oci://...` on the image.
   - Start the image with `docker run --rm ghcr.io/dummy-run/dummyrun:0.1.0 version`.
8. Check the release page. It shows the notes and 12 archives, which are 2 commands on 6 platforms. It also shows
   an SBOM for each archive, `checksums.txt` and `checksums.txt.sigstore.json`.
9. Announce it. Link the release page and the changelog section.

The download steps in `docs/INSTALL.md` and `site/content/start.md` tell the reader to take the newest release, so
they need no edit. Add a Homebrew section to `docs/INSTALL.md` when the tap is on.

## When a release fails

The `release` workflow checks the tag and the `main` commit before it builds anything. A run that fails that check leaves no draft and no image. Fix the cause and push the tag again.

A run that fails after the check leaves the release in draft, since only `publish` makes it public. The image gets its version, minor and `latest` tags once it is signed and attested. A failure before that leaves only an untagged digest. To retry:

1. If the `image` job gave the digest its tags, or `publish` failed after it, delete those tags in the package settings of `ghcr.io/dummy-run/dummyrun`.
2. Fix the cause.
3. Delete the draft release and its tag on GitHub.
4. Push the tag again on the fixed commit.

This cleanup applies only to a release that was never published. Never move a published tag. A published
release is final, so ship a new patch version instead.

## One-time setup

The repository owner does these steps once.

### The release environment

The `release` environment limits who can publish. Create it and restrict it to `v*` tags.

```sh
gh api -X PUT repos/dummy-run/dummy-run/environments/release \
  -F deployment_branch_policy[protected_branches]=false \
  -F deployment_branch_policy[custom_branch_policies]=true
gh api -X POST repos/dummy-run/dummy-run/environments/release/deployment-branch-policies \
  -f name='v*' -f type=tag
```

Add the owner as a required reviewer if you want a pause before each release. Add a tag ruleset for `v*` so
only maintainers can create and nobody can move or delete one.

### Homebrew, when you want it

The formula is off until these exist.

1. Create the public repository `dummy-run/homebrew-tap` with a `Formula` directory and a README.
2. Create a fine-grained personal access token with the tap repository as its only repository and the
   permission Contents: read and write. Give it an expiry and put the date in the team calendar.
3. Store it as the secret `HOMEBREW_TAP_TOKEN` of the `release` environment.
4. Set the repository variable `HOMEBREW_TAP_ENABLED` to `true`.

```sh
gh secret set HOMEBREW_TAP_TOKEN --env release --repo dummy-run/dummy-run
gh variable set HOMEBREW_TAP_ENABLED --body true --repo dummy-run/dummy-run
```

The next release then pushes `Formula/dummyrun.rb` to the tap. The formula installs the macOS and Linux
archives, and its test runs `dummyrun version`. The tap is updated while the release is still a draft. A failure in the `image` job or in `publish` can leave a formula that points at a draft. After you fix the failure and tag again, check that `Formula/dummyrun.rb` points at the new release before users install from the tap. Repair it if it is stale. Then add a Homebrew section to `docs/INSTALL.md`.

### Package visibility

GitHub creates the container package as private on its first push. After the first release, open the package
settings of `ghcr.io/dummy-run/dummyrun`, connect it to the repository and set its visibility to public.
Anonymous pulls need this, including the pulls of the hosted runner.

## Tooling versions

All actions are pinned by commit. GoReleaser is `v2.18.3`, cosign `v3.1.3` and syft `v1.55.0`, set at the top of
`release.yml`. Dependabot proposes action and base image updates weekly. Change a tool version in
`release.yml` and `release-check.yml` together.
