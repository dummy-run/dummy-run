# The Dummy Run image: the dummyrun binary with Chrome for Testing, chromedriver and Firefox.
#
# The build context is a directory that holds the release binaries as <os>/<arch>/dummyrun, for example
# linux/amd64/dummyrun and linux/arm64/dummyrun. The release workflow and docs/RELEASING.md say how to make it.
#
# The browser versions match .github/workflows/engine.yml. The release-check workflow fails when they differ.

# debian:trixie-slim, pinned by digest. Dependabot keeps it current.
FROM debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f AS browsers

ARG TARGETARCH
ARG CHROME_FOR_TESTING=155.0.8059.12
ARG FIREFOX_VERSION=157.0
ARG CHROME_SHA256_AMD64=a24546e0e6c763bf864861ce50498f38783663e24cd2af2a16798c25dc3a9489
ARG CHROME_SHA256_ARM64=bf94336ee2e3edb1279da28a74a744958ba10741e5994351610bc19028482b0b
ARG CHROMEDRIVER_SHA256_AMD64=ff08004e6f813c9a9084639b53f8faf0e99fa37afde7ce1c056090ee7735e138
ARG CHROMEDRIVER_SHA256_ARM64=511da4ed4025c7e3f213152d9a701adec84b8372d2e8af542ef67f10d44b8794
ARG FIREFOX_SHA256_AMD64=42f2c62a562316982ef5a796738c57602bf84a984f5c616e80bcff4627f78fff
ARG FIREFOX_SHA256_ARM64=73fc3d6f6f4d3fcdeee59db90156568af9959405120ad686e535f572995074d0
# Vale checks plain language with --plain-language. The version matches plain.ValeVersion in internal/plain.
ARG VALE_VERSION=3.24.0
ARG VALE_SHA256_AMD64=867534ddb678abca7f214bc4706ead0922bd5252ffe8035fc227013f66c8b214
ARG VALE_SHA256_ARM64=104cbd349279e50c4cb40adb70adf345d93cc4a3a0191f9e68bdccb97086b582
# The Nu HTML Checker checks HTML with --html. The version matches validate.VnuVersion in internal/validate. The jar
# is the same on both architectures, and it runs on a Java runtime cut down with jlink to the modules it needs.
ARG VNU_VERSION=26.10.12
ARG VNU_SHA256=3925f07364f5e5a111cef54a7baefa575b22d05c70f8ced048f0616d4579b7ab

SHELL ["/bin/bash", "-euo", "pipefail", "-c"]
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl unzip xz-utils \
 && rm -rf /var/lib/apt/lists/*

# Each download is checked against a hash kept in this file, so a changed file upstream fails the build.
RUN case "${TARGETARCH}" in \
      amd64) cft=linux64;     cft_zip=linux64;     ff=linux-x86_64;  vale=Linux_64-bit; chrome_sha="${CHROME_SHA256_AMD64}"; driver_sha="${CHROMEDRIVER_SHA256_AMD64}"; ff_sha="${FIREFOX_SHA256_AMD64}"; vale_sha="${VALE_SHA256_AMD64}" ;; \
      arm64) cft=linux-arm64; cft_zip=linux-arm64; ff=linux-aarch64; vale=Linux_arm64;  chrome_sha="${CHROME_SHA256_ARM64}"; driver_sha="${CHROMEDRIVER_SHA256_ARM64}"; ff_sha="${FIREFOX_SHA256_ARM64}"; vale_sha="${VALE_SHA256_ARM64}" ;; \
      *) echo "unsupported architecture ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    base="https://storage.googleapis.com/chrome-for-testing-public/${CHROME_FOR_TESTING}/${cft}"; \
    curl -fsSL -o chrome.zip "${base}/chrome-${cft_zip}.zip"; \
    curl -fsSL -o chromedriver.zip "${base}/chromedriver-${cft_zip}.zip"; \
    curl -fsSL -o firefox.tar.xz "https://download-installer.cdn.mozilla.net/pub/firefox/releases/${FIREFOX_VERSION}/${ff}/en-US/firefox-${FIREFOX_VERSION}.tar.xz"; \
    echo "${chrome_sha}  chrome.zip" | sha256sum -c -; \
    echo "${driver_sha}  chromedriver.zip" | sha256sum -c -; \
    curl -fsSL -o vale.tar.gz "https://github.com/vale-cli/vale/releases/download/v${VALE_VERSION}/vale_${VALE_VERSION}_${vale}.tar.gz"; \
    echo "${ff_sha}  firefox.tar.xz" | sha256sum -c -; \
    echo "${vale_sha}  vale.tar.gz" | sha256sum -c -; \
    mkdir -p /opt/chrome /opt/chromedriver /opt/firefox /opt/vale; \
    tar -xzf vale.tar.gz -C /opt/vale vale; \
    unzip -q chrome.zip -d /tmp/chrome; \
    mv /tmp/chrome/chrome-*/* /opt/chrome/; \
    unzip -q chromedriver.zip -d /tmp/chromedriver; \
    mv /tmp/chromedriver/chromedriver-*/chromedriver /opt/chromedriver/chromedriver; \
    tar -xJf firefox.tar.xz -C /opt/firefox --strip-components=1; \
    find /opt/chrome/locales -name '*.pak' ! -name 'en-US.pak' -delete

RUN apt-get update \
 && apt-get install -y --no-install-recommends openjdk-21-jdk-headless binutils \
 && rm -rf /var/lib/apt/lists/* \
 && curl -fsSL -o vnu.tgz "https://registry.npmjs.org/vnu-jar/-/vnu-jar-${VNU_VERSION}.tgz" \
 && echo "${VNU_SHA256}  vnu.tgz" | sha256sum -c - \
 && mkdir -p /opt/vnu/bin /tmp/vnu \
 && tar -xzf vnu.tgz -C /tmp/vnu \
 && mv /tmp/vnu/package/build/dist/vnu.jar /opt/vnu/vnu.jar \
 && mv /tmp/vnu/package/LICENSE /opt/vnu/LICENSE \
 && jlink --add-modules java.base,java.datatransfer,java.xml,java.prefs,java.desktop,java.logging,java.management,java.security.sasl,java.naming,java.sql,jdk.crypto.ec \
      --strip-debug --no-man-pages --no-header-files --compress=zip-9 --output /opt/vnu/jre \
 && printf '#!/bin/sh\nexec /opt/vnu/jre/bin/java -Xss1m -jar /opt/vnu/vnu.jar "$@"\n' > /opt/vnu/bin/vnu \
 && chmod 0755 /opt/vnu/bin/vnu

FROM debian:trixie-slim@sha256:a29215f6a35e51e22adffa17f89e9d2ef06214e64a2bad10d765c46aea49f11f

# Libraries that Chrome for Testing and Firefox load, fonts for screenshots, and tini as the init process.
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates tini \
      fonts-liberation fonts-dejavu-core fonts-noto-color-emoji \
      libasound2t64 libatk-bridge2.0-0t64 libatk1.0-0t64 libcairo2 libcups2t64 libdbus-1-3 libdbus-glib-1-2 \
      libdrm2 libgbm1 libgtk-3-0t64 libnspr4 libnss3 libpango-1.0-0 libx11-6 libx11-xcb1 libxcb1 \
      libxcomposite1 libxdamage1 libxext6 libxfixes3 libxkbcommon0 libxrandr2 libxtst6 \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --gid 10001 dummyrun \
 && useradd --uid 10001 --gid 10001 --create-home --home-dir /home/dummyrun --shell /usr/sbin/nologin dummyrun \
 && mkdir -p /data \
 && chown dummyrun:dummyrun /data

COPY --from=browsers /opt/chrome /opt/chrome
COPY --from=browsers /opt/chromedriver /opt/chromedriver
COPY --from=browsers /opt/firefox /opt/firefox
COPY --from=browsers /opt/vale /opt/vale
COPY --from=browsers /opt/vnu /opt/vnu
ARG TARGETOS
ARG TARGETARCH
COPY --chmod=0755 ${TARGETOS}/${TARGETARCH}/dummyrun /usr/local/bin/dummyrun

# The container is the sandbox, so the browsers' own sandboxes are off. See docs/INSTALL.md.
ENV DUMMYRUN_CHROME=/opt/chrome/chrome \
    DUMMYRUN_CHROMEDRIVER=/opt/chromedriver/chromedriver \
    DUMMYRUN_FIREFOX=/opt/firefox/firefox \
    DUMMYRUN_VALE=/opt/vale/vale \
    DUMMYRUN_VNU=/opt/vnu/bin/vnu \
    DUMMYRUN_NO_SANDBOX=1 \
    HOME=/home/dummyrun

LABEL org.opencontainers.image.title="Dummy Run" \
      org.opencontainers.image.description="AI personas try your app in Chrome or Firefox and report what broke." \
      org.opencontainers.image.source="https://github.com/dummy-run/dummy-run" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.documentation="https://github.com/dummy-run/dummy-run/blob/main/docs/INSTALL.md"

USER 10001:10001
WORKDIR /data
VOLUME ["/data"]
ENTRYPOINT ["/usr/bin/tini", "--", "dummyrun"]
CMD ["--help"]
