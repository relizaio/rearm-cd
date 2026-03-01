FROM --platform=$BUILDPLATFORM golang:1.25.6-alpine3.23@sha256:98e6cffc31ccc44c7c15d83df1d69891efee8115a5bb7ede2bf30a38af3e3c92 AS build-stage
WORKDIR /build
RUN apk add --update --no-cache cosign unzip 
ADD https://dl.k8s.io/v1.33.7/kubernetes-client-linux-${TARGETARCH}.tar.gz ./kubernetes-client-linux-${TARGETARCH}.tar.gz
RUN sha512sum -c tools.${TARGETARCH}.sha512
RUN tar -xzf kubernetes-client-linux-${TARGETARCH}.tar.gz && \
    mv kubernetes/client/bin/kubectl kubectl

FROM registry.relizahub.com/library/rearm-cli:26.02.0@sha256:340190f8489054d7eafe1a8ea38ebc071443c79f1dd44eec9cbeca036b1ea866 AS package
ARG CI_ENV=noci
ARG GIT_COMMIT=git_commit_undefined
ARG GIT_BRANCH=git_branch_undefined
ARG VERSION=not_versioned
ARG TARGETARCH
USER root
RUN apk add bash jq
RUN mkdir /resources && chown apprunner:apprunner /resources
COPY --chown=apprunner:apprunner entrypoint.sh /
RUN chmod 0500 /entrypoint.sh
ENV NAMESPACE allnamespaces
ENV SENDER_ID default
COPY --from=build-stage --chown=apprunner:apprunner /build/kubectl /bin/kubectl
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