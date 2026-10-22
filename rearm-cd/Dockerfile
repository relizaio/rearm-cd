FROM --platform=$BUILDPLATFORM golang:1.26.10-alpine3.24@sha256:5cb67f19c5e7994dcb80c074f7cfca8aeeada45bbdca035f73df269cefdcce61 AS build-stage
WORKDIR /build
RUN apk add --update --no-cache cosign unzip 
ENV CGO_ENABLED=0
COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN go test ./...
RUN go test -count=1 -tags trustgate -run TestTrustGate ./ci/trust-gate/
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build
ADD https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.40.0/kubeseal-0.40.0-linux-${TARGETARCH}.tar.gz ./kubeseal-0.40.0-linux-${TARGETARCH}.tar.gz
ADD https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.40.0/kubeseal-0.40.0-linux-${TARGETARCH}.tar.gz.sig ./kubeseal-0.40.0-linux-${TARGETARCH}.tar.gz.sig
ADD https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.40.0/cosign.pub ./cosign.pub
ADD https://get.helm.sh/helm-v4.3.0-linux-${TARGETARCH}.tar.gz ./helm-v4.3.0-linux-${TARGETARCH}.tar.gz
ADD https://dl.k8s.io/v1.35.9/bin/linux/${TARGETARCH}/kubectl ./kubectl
# rearm-cli >= 26.07.3 is REQUIRED: cli/deployfailure.go shells out to
# `rearm devops instevent`, which does not exist in earlier releases. On an
# older binary the reporting call just fails and is warn-logged, so deployment
# failures silently stop reaching the Instance view -- bump this pin (and both
# tools.<arch>.sha256 files) whenever a new agent-facing verb is relied upon.
ADD https://d7ge14utcyki8.cloudfront.net/rearm-download/26.10.2/rearm-26.10.2-linux-${TARGETARCH}.zip ./rearm-26.10.2-linux-${TARGETARCH}.zip
RUN sha256sum -c tools.${TARGETARCH}.sha256
RUN cosign verify-blob --key cosign.pub --signature kubeseal-0.40.0-linux-${TARGETARCH}.tar.gz.sig kubeseal-0.40.0-linux-${TARGETARCH}.tar.gz
RUN tar -xzvf kubeseal-0.40.0-linux-${TARGETARCH}.tar.gz
RUN tar -xzvf helm-v4.3.0-linux-${TARGETARCH}.tar.gz
RUN unzip rearm-26.10.2-linux-${TARGETARCH}.zip
RUN apk add --no-cache openssl && sh ci/trust-gate/selftest.sh
RUN for b in rearm kubectl kubeseal linux-${TARGETARCH}/helm; do go version -m "$b" | head -n 1; done

FROM alpine:3.24.3@sha256:b3413572228e34bd1c9245f5565e6cd127005655e8a7899aef33d8e83255c9e6 AS release-stage
ARG TARGETARCH
ARG CI_ENV=noci
ARG GIT_COMMIT=git_commit_undefined
ARG GIT_BRANCH=git_branch_undefined
ARG VERSION=not_versioned
ENV ARGO_HELM_VERSION=5.51.6
RUN apk upgrade --no-cache ca-certificates ca-certificates-bundle && apk info -e -v ca-certificates-bundle && apk add --update --no-cache openssl
RUN mkdir /app && adduser -u 1000 -D apprunner && chown apprunner:apprunner -R /app
USER apprunner
RUN mkdir -p /app/workspace && mkdir /app/tools
USER root
COPY --from=build-stage --chown=apprunner:apprunner /build/rearm-cd /app/rearm-cd
COPY --from=build-stage --chown=apprunner:apprunner /build/kubeseal /app/tools/kubeseal
COPY --from=build-stage --chown=apprunner:apprunner /build/kubectl /app/tools/kubectl
COPY --from=build-stage --chown=apprunner:apprunner /build/rearm /app/tools/rearm
COPY --from=build-stage --chown=apprunner:apprunner /build/linux-${TARGETARCH}/helm /app/tools/helm
COPY --chown=apprunner:apprunner entrypoint.sh /entrypoint.sh

RUN chmod 0550 /entrypoint.sh && chmod 0550 /app/tools/kubectl
USER apprunner
RUN echo "version=$VERSION" > /app/version && echo "commit=$GIT_COMMIT" >> /app/version && echo "branch=$GIT_BRANCH" >> /app/version

LABEL org.opencontainers.image.revision=$GIT_COMMIT
LABEL git_branch=$GIT_BRANCH
LABEL ci_environment=$CI_ENV
LABEL org.opencontainers.image.version=$VERSION

ENTRYPOINT ["/entrypoint.sh"]

# Trust-store gate: the bundle this image ships must refuse the compromised
# eMudhra chains (ci/trust-gate/). The release stage copies the marker, which is
# what makes BuildKit build this stage. The Go probe ran in build-stage.
FROM release-stage AS trust-gate
USER root
RUN apk add --no-cache curl openssl
COPY ci/trust-gate/ /gate/
RUN sh /gate/trust-gate.sh && echo "trust-gate ok $(date -u +%FT%TZ)" > /gate/ok

FROM release-stage AS release
COPY --from=trust-gate /gate/ok /trust-gate-ok
