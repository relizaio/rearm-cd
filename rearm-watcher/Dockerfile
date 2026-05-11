FROM --platform=$BUILDPLATFORM golang:1.26.3-alpine3.23@sha256:91eda9776261207ea25fd06b5b7fed8d397dd2c0a283e77f2ab6e91bfa71079d AS build-stage
WORKDIR /build
ARG TARGETARCH
COPY . .
RUN apk add --update --no-cache cosign unzip 
ADD https://dl.k8s.io/v1.34.7/bin/linux/${TARGETARCH}/kubectl ./kubectl
ADD https://d7ge14utcyki8.cloudfront.net/rearm-download/26.05.5/rearm-26.05.5-linux-${TARGETARCH}.zip ./rearm-26.05.5-linux-${TARGETARCH}.zip
RUN sha256sum -c tools.${TARGETARCH}.sha256
RUN unzip rearm-26.05.5-linux-${TARGETARCH}.zip

FROM alpine:3.23.4@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11 AS release-stage
ARG TARGETARCH
ARG CI_ENV=noci
ARG GIT_COMMIT=git_commit_undefined
ARG GIT_BRANCH=git_branch_undefined
ARG VERSION=not_versioned
USER root
RUN apk add --update --no-cache openssl bash jq && \
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