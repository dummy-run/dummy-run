# Install

Dummy Run ships as binaries, a container image and Go source. Every release is signed. Verify what you
download before you run it.

## Binaries

Each release on [GitHub Releases](https://github.com/dummy-run/dummy-run/releases) has two commands for
Linux, macOS and Windows on amd64 and arm64.

| Command | What it does |
|---|---|
| `dummyrun` | Plays personas in a browser, sweeps pages for accessibility problems and reports coverage |
| `dummyrun-otel` | Sends run records to an OpenTelemetry backend. See [the Grafana guide](integrations/grafana.md) |

The archives are `dummyrun_<version>_<os>_<arch>.tar.gz`, or `.zip` on Windows. Each holds the command,
`LICENSE`, `NOTICE` and `README.md`. `checksums.txt` lists the SHA-256 of every archive and SBOM, and each
archive has an SPDX SBOM next to it. The engine needs a browser, so read [ENGINE.md](ENGINE.md) for Chrome
for Testing and Firefox. The container image already has both.

### Verify a download

Set the version and download the archive, `checksums.txt` and `checksums.txt.sigstore.json`.

```sh
VERSION=0.1.0
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/dummy-run/dummy-run/.github/workflows/release.yml@refs/tags/v${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --ignore-missing -c checksums.txt
```

The first command proves that the release workflow of this repository signed the checksums for that tag. The
second proves that your archive is one of the files listed. On macOS use `shasum -a 256 --ignore-missing -c
checksums.txt`.

The archives also carry SLSA build provenance from GitHub. With the [GitHub CLI](https://cli.github.com/):

```sh
gh attestation verify dummyrun_${VERSION}_linux_amd64.tar.gz --repo dummy-run/dummy-run
```

This proves which workflow run, commit and tag built the file. The builds are reproducible. Building the same
commit with the same Go version gives the same binaries, since the build uses `-trimpath` and takes its
timestamp from the commit.

## Container image

The image is `ghcr.io/dummy-run/dummyrun`. It is built for `linux/amd64` and `linux/arm64`. Both have the
`dummyrun` command and Chrome for Testing with its chromedriver. They also have Firefox, fonts for screenshots
and [tini](https://github.com/krallin/tini) as the init process. The versions of both browsers match the ones CI tests.

| Tag | Meaning |
|---|---|
| `0.1.0` | That release. Use this one for anything repeatable |
| `0.1` | The newest patch release of that minor version |
| `latest` | The newest release that is not a pre-release |

The image digest is the safest reference. `docker buildx imagetools inspect ghcr.io/dummy-run/dummyrun:0.1.0`
prints it.

```sh
docker run --rm ghcr.io/dummy-run/dummyrun:0.1.0 version
docker run --rm ghcr.io/dummy-run/dummyrun:0.1.0 a11y https://example.com
```

### Verify the image

```sh
cosign verify ghcr.io/dummy-run/dummyrun:${VERSION} \
  --certificate-identity "https://github.com/dummy-run/dummy-run/.github/workflows/release.yml@refs/tags/v${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
gh attestation verify oci://ghcr.io/dummy-run/dummyrun:${VERSION} --repo dummy-run/dummy-run
```

The second command checks the SLSA provenance. The release also attaches an SBOM attestation, which
`gh attestation verify --predicate-type https://spdx.dev/Document/v2.3` checks. The SBOM describes the
`linux/amd64` image.

### Run a persona in the container

```sh
DEPLOYMENT=my-deployment
docker run --rm \
  -v "$PWD/casts:/cast:ro" \
  -v "$PWD/results:/data" \
  -e DUMMYRUN_AZURE_ENDPOINT -e AZURE_CLIENT_ID -e AZURE_TENANT_ID -e AZURE_CLIENT_SECRET \
  ghcr.io/dummy-run/dummyrun:${VERSION} run \
  --cast /cast/dummyrun-dev --persona tom-developer --scenario find-the-quickstart \
  --target /cast/target.json --out /data/runs --browser firefox \
  --provider azure --model "$DEPLOYMENT" --region uksouth
```

### What the image contains

| Item | Value |
|---|---|
| User | `dummyrun`, uid and gid 10001, home `/home/dummyrun`. The image never runs as root |
| Entrypoint | `tini -- dummyrun`, so the arguments are a `dummyrun` command. With none it prints the help |
| Working directory | `/data`, declared as a volume |
| Commands | Only `dummyrun`. Use the binary release for `dummyrun-otel` |
| Fonts | Liberation, DejaVu and Noto Color Emoji. Pages in Chinese, Japanese or Korean show boxes in screenshots |
| Healthcheck | None. The container runs one command and exits |

### Environment variables

| Variable | Set by the image | Meaning |
|---|---|---|
| `DUMMYRUN_CHROME` | `/opt/chrome/chrome` | Chrome for Testing |
| `DUMMYRUN_CHROMEDRIVER` | `/opt/chromedriver/chromedriver` | The matching chromedriver |
| `DUMMYRUN_FIREFOX` | `/opt/firefox/firefox` | Firefox |
| `DUMMYRUN_NO_SANDBOX` | `1` | Switches off the browsers' own sandboxes, since the container is the sandbox. Unset it only where the container allows user namespaces |
| `DUMMYRUN_AZURE_ENDPOINT` | | The Azure OpenAI endpoint, such as `https://<resource>.openai.azure.com` |
| `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_CLIENT_SECRET` | | A service principal for Azure. With a managed identity or workload identity, set only the variables that identity needs. [PROVIDERS.md](PROVIDERS.md) lists the sign-in routes |
| `ANTHROPIC_API_KEY`, `GEMINI_API_KEY` | | Keys for those providers, or the variable named by `--api-key-env` |
| `GOOGLE_CLOUD_PROJECT` | | The Google Cloud project for the `vertex` provider |

Pass secrets as environment variables from your platform's secret store. Never bake them into an image or
put them on a command line. A key is read when the run starts and never written to a run record.

### Volumes and write access

| Path | Use | Access |
|---|---|---|
| `/data` | Run records, screenshots and reports. Pass `--out /data/runs`. Mount a volume here to keep them | Read and write for uid 10001 |
| `/cast` | A cast directory and target configuration. The image holds none, so mount yours | Read only is enough |
| `/tmp` | Browser profiles, removed when a run ends | Read and write |

With a bind mount, make the host directory writable for uid 10001, or pass `--user` with a uid that owns it.
The container can run with `--read-only` when `/tmp` and `/data` are writable, for example with `--tmpfs /tmp`.

### Hosted runs

The hosted free-scan runner runs this image as a job, such as an Azure Container Apps job. It passes the
command in the job's arguments, for example `a11y --json /data/report.json <url>`. It mounts storage at
`/data`, uses a managed identity for Azure and sets `DUMMYRUN_AZURE_ENDPOINT` as a plain variable. Give the job
at least 2 vCPU and 4 GiB of memory, since Chrome needs room. Reference the image by digest so a job never
changes without a deliberate update.

## Go install

You need Go 1.27.

```sh
go install github.com/dummy-run/dummy-run/cmd/dummyrun@latest
go install github.com/dummy-run/dummy-run/integrations/grafana/exporter/cmd/dummyrun-otel@latest
```

A build made this way reports the version `0.1.0-dev` for `dummyrun` and `dev` for `dummyrun-otel`. Use a
release binary when you need to know which release you run.
