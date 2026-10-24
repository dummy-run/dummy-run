# Install

You can have a first run in a few minutes. Download Dummy Run, let it fetch a browser for you, give it one
API key and point it at your site. Every release is signed, so you can check what you download before you run it.

## Your first run

1. Download the release for your machine from [GitHub Releases](https://github.com/dummy-run/dummy-run/releases),
   unpack it and put `dummyrun` on your path. On a Mac with Apple silicon:

   ```sh
   VERSION=...   # the newest release number on the releases page
   curl -fsSLO "https://github.com/dummy-run/dummy-run/releases/download/v${VERSION}/dummyrun_${VERSION}_darwin_arm64.tar.gz"
   tar -xzf "dummyrun_${VERSION}_darwin_arm64.tar.gz"
   ```

   The other archives are listed under [Binaries](#binaries). If you have Go, `go install` works instead (see
   [Go install](#go-install)). Check the install:

   <!-- doctest -->
   ```sh
   dummyrun version
   ```

2. Fetch a browser. This downloads the Chrome for Testing that Dummy Run was tested with, and the chromedriver
   that matches it, into your user cache. You usually do it once, and there is nothing to match by hand.

   ```sh
   dummyrun browsers install
   ```

   <!-- doctest -->
   ```sh
   dummyrun browsers list
   ```

3. Give it a model. Set one key and Dummy Run finds it. Without `--provider` it looks for `OPENAI_API_KEY`
   first and `ANTHROPIC_API_KEY` second. A running [Ollama](https://ollama.com/) comes third. If you have
   `DUMMYRUN_AZURE_ENDPOINT` set, Azure goes ahead of all three. It prints what it chose on the first line. [PROVIDERS.md](PROVIDERS.md) lists the models it picks and how to choose your own.

   ```sh
   export ANTHROPIC_API_KEY=...
   ```

4. Point it at your site.

   ```sh
   dummyrun try https://your-site
   ```

   A first-time visitor plays through your page for a few minutes. It reads and clicks in read-only mode: the
   browser blocks every request that could change data and every request to another host. You get the findings
   in your terminal. You also get the visitor's own report, a screenshot of every step and the run record, each
   with its path.

   <!-- doctest -->
   ```sh
   dummyrun try -h
   ```

Azure OpenAI, Gemini, Bedrock and local models all work too. [PROVIDERS.md](PROVIDERS.md) covers each of them.

## Where the browsers live

`dummyrun browsers install` puts Chrome for Testing in `os.UserCacheDir()/dummyrun/browsers/<version>`. On
macOS that is `~/Library/Caches/dummyrun/browsers`, on Linux `~/.cache/dummyrun/browsers`. Dummy Run finds it
there by itself. Each download is checked against a hash that ships inside `dummyrun`, and a file that
doesn't match is not unpacked.

| Command | What it does |
|---|---|
| `dummyrun browsers install` | Installs Chrome for Testing and its chromedriver. Run it again and nothing happens |
| `dummyrun browsers install firefox` | Uses the Firefox already on your machine. On Linux with none, downloads the pinned release |
| `dummyrun browsers install all` | Both of the above |
| `dummyrun browsers list` | Shows the pinned versions, where each is, and the cache folder |

If you set `DUMMYRUN_CHROME`, `DUMMYRUN_CHROMEDRIVER` or `DUMMYRUN_FIREFOX`, those win over the cache.
`DUMMYRUN_BROWSER_CACHE` moves the cache. With no network, the command says so and names itself, so you can
run it again when you're online. The container image already has both browsers.

## Binaries

Each release on [GitHub Releases](https://github.com/dummy-run/dummy-run/releases) has two commands for
Linux, macOS and Windows on amd64 and arm64.

| Command | What it does |
|---|---|
| `dummyrun` | Plays personas in a browser, sweeps pages for accessibility problems and reports coverage |
| `dummyrun-otel` | Sends run records to an OpenTelemetry backend. See [the Grafana guide](integrations/grafana.md) |

The archives are `dummyrun_<version>_<os>_<arch>.tar.gz`, or `.zip` on Windows. Each holds the command,
`LICENSE`, `NOTICE` and `README.md`. `checksums.txt` lists the SHA-256 of every archive and SBOM, and each
archive has an SPDX SBOM next to it. The engine needs a browser, and `dummyrun browsers install` fetches one
(see above). The container image already has both.

### Verify a download

Set the version and download the archive, `checksums.txt` and `checksums.txt.sigstore.json`.

```sh
VERSION=0.1.1
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
| `0.1.1` | That release. Use this one for anything repeatable |
| `0.1` | The newest patch release of that minor version |
| `latest` | The newest release that is not a pre-release |

The image digest is the safest reference. `docker buildx imagetools inspect ghcr.io/dummy-run/dummyrun:0.1.1`
prints it.

```sh
docker run --rm ghcr.io/dummy-run/dummyrun:0.1.1 version
docker run --rm ghcr.io/dummy-run/dummyrun:0.1.1 a11y https://example.com
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

### Try your site from the container

The image has both browsers, so there's nothing to install. Pass one key and the address:

```sh
docker run --rm -e ANTHROPIC_API_KEY -v "$PWD/results:/data" \
  ghcr.io/dummy-run/dummyrun:${VERSION} try https://your-site --out /data/runs
```

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
| `DUMMYRUN_VALE` | `/opt/vale/vale` | Vale, the prose checker `dummyrun a11y --plain-language` runs |
| `DUMMYRUN_WEBAPPANALYZER` | not set | Your copy of webappanalyzer, whose fingerprints a run adds to its own for `target.tech` |
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

You can also run the image as a job on a hosted platform, such as an Azure Container Apps job. Pass the
command in the job's arguments, for example `a11y --json /data/report.json <url>`. Mount storage at `/data`. On Azure, a
managed identity saves you a secret, and `DUMMYRUN_AZURE_ENDPOINT` can be a plain variable. Give the job at
least 2 vCPU and 4 GiB of memory, because Chrome needs room. Reference the image by digest, so the job only
changes when you update it.

## Go install

You'll need Go 1.27.1 or later.

```sh
go install github.com/dummy-run/dummy-run/cmd/dummyrun@latest
go install github.com/dummy-run/dummy-run/integrations/grafana/exporter/cmd/dummyrun-otel@latest
```

A build made this way reports a `-dev` version for `dummyrun` and `dev` for `dummyrun-otel`. Use a
release binary when you need to know which release you run.
