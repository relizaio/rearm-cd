FROM --platform=$BUILDPLATFORM golang:1.26.10-alpine3.24@sha256:5cb67f19c5e7994dcb80c074f7cfca8aeeada45bbdca035f73df269cefdcce61 AS build-stage
WORKDIR /build
ARG TARGETARCH
COPY . .
RUN apk add --update --no-cache cosign unzip 
ADD https://dl.k8s.io/v1.36.5/bin/linux/${TARGETARCH}/kubectl ./kubectl
ADD https://d7ge14utcyki8.cloudfront.net/rearm-download/26.10.17/rearm-26.10.17-linux-${TARGETARCH}.zip ./rearm-26.10.17-linux-${TARGETARCH}.zip
RUN sha256sum -c tools.${TARGETARCH}.sha256
RUN unzip rearm-26.10.17-linux-${TARGETARCH}.zip
RUN go version -m rearm | head -n 1
RUN apk add --no-cache openssl && sh ci/trust-gate/selftest.sh

FROM alpine:3.24.3@sha256:b3413572228e34bd1c9245f5565e6cd127005655e8a7899aef33d8e83255c9e6 AS release-stage
ARG TARGETARCH
ARG CI_ENV=noci
ARG GIT_COMMIT=git_commit_undefined
ARG GIT_BRANCH=git_branch_undefined
ARG VERSION=not_versioned
USER root
# No openssl: entrypoint.sh uses only kubectl, jq and rearm.
RUN apk upgrade --no-cache ca-certificates ca-certificates-bundle && apk info -e -v ca-certificates-bundle && apk add --update --no-cache bash jq && \
    mkdir /app && adduser -u 1000 -D apprunner && chown apprunner:apprunner -R /app && \
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
