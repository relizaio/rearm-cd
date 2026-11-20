FROM --platform=$BUILDPLATFORM golang:1.26.11-alpine3.24@sha256:ecb67f987aa1a57794c91d19ade7ae0209f2f0f5496aaef9e6ae85ddf92d02ab AS build-stage
WORKDIR /build
ARG TARGETARCH
COPY . .
RUN apk add --update --no-cache cosign unzip 
ADD https://dl.k8s.io/v1.36.6/bin/linux/${TARGETARCH}/kubectl ./kubectl
ADD https://d7ge14utcyki8.cloudfront.net/rearm-download/26.11.10/rearm-26.11.10-linux-${TARGETARCH}.zip ./rearm-26.11.10-linux-${TARGETARCH}.zip
RUN sha256sum -c tools.${TARGETARCH}.sha256
RUN unzip rearm-26.11.10-linux-${TARGETARCH}.zip
RUN go version -m rearm | head -n 1
RUN apk add --no-cache openssl && sh ci/trust-gate/selftest.sh

FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf AS runtime-base
# Upgrade every package: alpine:3.24.4 still ships zlib 1.3.2-r1 (CVE-2026-4738) and libssl3/libcrypto3
# 3.5.9-r0; the build fails unless each reached its fixed release, and unless ca-certificates-bundle is
# the eMudhra-fixed 20261020-r0 or later (a missing package or an incomparable version fails too).
# No openssl: entrypoint.sh uses only kubectl, jq and rearm.
RUN apk upgrade --no-cache && apk add --no-cache bash jq && \
    for p in zlib:1.4-r0 libssl3:3.5.9-r1 libcrypto3:3.5.9-r1 ca-certificates-bundle:20261020-r0; do \
        v=$(apk info -e -v "${p%%:*}" | sed "s/^${p%%:*}-//") && \
        apk version -t "$v" "${p#*:}" | grep -qx '[=>]' || { echo "${p%%:*} '$v' is older than ${p#*:}" >&2; exit 1; }; \
    done

# Flattened onto scratch: the image SBOM (cdxgen on the `docker save` tarball, in rearm-docker-action) reads
# every layer, so the base layer's superseded packages would otherwise still be reported after the upgrade
# above. The copy drops Alpine's image config (PATH and CMD ["/bin/sh"]): PATH is restored here, the
# ENTRYPOINT below replaces CMD.
FROM scratch AS release-stage
COPY --from=runtime-base / /
ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ARG TARGETARCH
ARG CI_ENV=noci
ARG GIT_COMMIT=git_commit_undefined
ARG GIT_BRANCH=git_branch_undefined
ARG VERSION=not_versioned
USER root
RUN mkdir /app && adduser -u 1000 -D apprunner && chown apprunner:apprunner -R /app && \
    mkdir -p /resources/ns && chown apprunner:apprunner /resources && chown apprunner:apprunner /resources/ns
COPY --chown=apprunner:apprunner entrypoint.sh /
RUN chmod 0550 /entrypoint.sh
ENV NAMESPACE=allnamespaces
ENV SENDER_ID=default
COPY --from=build-stage --chown=apprunner:apprunner /build/kubectl /bin/kubectl
COPY --from=build-stage --chown=apprunner:apprunner /build/rearm /bin/rearm
RUN chmod 0550 /bin/kubectl /bin/rearm
USER apprunner
RUN echo "{}" > /resources/images
LABEL git_commit=$GIT_COMMIT
LABEL git_branch=$GIT_BRANCH
LABEL ci_environment=$CI_ENV
LABEL org.opencontainers.image.version=$VERSION
LABEL org.opencontainers.image.vendor="Reliza Incorporated"
LABEL org.opencontainers.image.title="ReARM Watcher"
LABEL org.opencontainers.image.source="https://github.com/relizaio/rearm-cd"
LABEL org.opencontainers.image.license="MIT"
LABEL org.opencontainers.image.url="https://rearmhq.com"
LABEL org.opencontainers.image.base.name="registry.relizahub.com/library/rearm-watcher-app"
ENTRYPOINT ["/entrypoint.sh"]

# Trust-store gate: the bundle this image ships must refuse the compromised
# eMudhra chains (ci/trust-gate/). The release stage copies the marker, which is
# what makes BuildKit build this stage. The watcher compiles nothing, so there
# is no Go probe; build-stage prints the toolchain of the rearm CLI it ships.
FROM release-stage AS trust-gate
USER root
RUN apk add --no-cache curl openssl
COPY ci/trust-gate/ /gate/
RUN sh /gate/trust-gate.sh && echo "trust-gate ok $(date -u +%FT%TZ)" > /gate/ok

FROM release-stage AS release
COPY --from=trust-gate /gate/ok /trust-gate-ok
