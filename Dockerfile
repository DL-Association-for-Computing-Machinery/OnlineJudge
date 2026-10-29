FROM golang:1.26.10-bookworm@sha256:386f0cbb8fb3402ff5a15048c96731e6ff37142d31182f3b72306359a1f5e9b7 AS backend-deps
WORKDIR /src
ENV GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download && go mod verify
FROM backend-deps AS backend-source
COPY cmd/ cmd/
COPY internal/ internal/
COPY db/ db/
COPY tests/ tests/
COPY deploy/experiments/go-judge.Dockerfile deploy/experiments/go-judge.Dockerfile

FROM backend-source AS host-build
ARG HOSTOS=darwin
ARG HOSTARCH=arm64
RUN set -eu; mkdir /out; for app in api worker worker-api migrate; do \
    GOOS=$HOSTOS GOARCH=$HOSTARCH CGO_ENABLED=0 go build -trimpath -o /out/$app ./cmd/$app; done \
    && GOOS=$HOSTOS GOARCH=$HOSTARCH CGO_ENABLED=0 go test -c -tags=sandbox -o /out/sandbox.test ./tests/sandbox

FROM scratch AS host-artifacts
COPY --from=host-build /out/ /

FROM backend-deps AS sqlc-build
RUN GOBIN=/out go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

# Scanner tools are development-only; runtime images do not inherit them.
FROM backend-deps AS security-tools
ENV GOTELEMETRY=off
RUN GOBIN=/usr/local/bin go install github.com/zricethezav/gitleaks/v8@v8.30.1
RUN GOBIN=/usr/local/bin go install golang.org/x/vuln/cmd/govulncheck@v1.9.0

FROM backend-source AS backend-audit
COPY --from=security-tools /usr/local/bin/govulncheck /usr/local/bin/govulncheck
ENV GOTELEMETRY=off
CMD ["govulncheck", "-show", "verbose", "./..."]

FROM backend-deps AS quality-tools
ENV GOTELEMETRY=off
RUN GOBIN=/out go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

FROM backend-source AS backend-quality
COPY --from=quality-tools /out/golangci-lint /usr/local/bin/golangci-lint
COPY .golangci.yml ./
COPY scripts/check-go.sh scripts/check-go.sh
COPY scripts/test-go-quality.sh scripts/test-go-quality.sh
RUN cd tests/sessionexperiment && go mod download && go mod verify
ENV GOTELEMETRY=off

FROM postgres:14-bookworm@sha256:a389ed49344afc8f4444efd4eb512965df07aaa6ed84b239b54a23246e9fe347 AS toolchain
COPY --from=backend-source /usr/local/go /usr/local/go
COPY --from=backend-quality /go/pkg/mod /go/pkg/mod
COPY --from=backend-quality /src /src
COPY --from=backend-quality /usr/local/bin/golangci-lint /usr/local/bin/golangci-lint
COPY --from=sqlc-build /out/sqlc /usr/local/bin/sqlc
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev python3 diffutils \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY scripts/ scripts/
COPY sqlc.yaml ./
ENV PATH="/usr/local/go/bin:${PATH}" GOTOOLCHAIN=local GOCACHE=/tmp/go-build GOMODCACHE=/go/pkg/mod
RUN mkdir /tmp/go-build && chown -R postgres:postgres /src /go/pkg/mod /tmp/go-build
USER postgres
ENTRYPOINT ["sh", "scripts/toolchain.sh"]
CMD ["backend"]

FROM backend-source AS backend-build
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api

FROM alpine:3.23@sha256:ff7f0c727dd796aa36787bfcd9a64bf35a6ce72a73451b3359fde849b6703d63 AS api
COPY --from=backend-build /out/api /usr/local/bin/oj-api
USER 65532:65532
ENV OJ_API_ADDR=0.0.0.0:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/oj-api"]

FROM node:24.19.0-bookworm-slim@sha256:a9f5f7c91a432850b2a8a7797adf5eadb6c733ceed61167806cee7ea7fbc29df AS frontend-source
WORKDIR /src/web
RUN npm install --global pnpm@11.25.0
COPY web/package.json web/pnpm-lock.yaml web/.npmrc ./
RUN pnpm install --frozen-lockfile --ignore-scripts
COPY web/ ./

FROM frontend-source AS frontend-build
RUN pnpm build

FROM nginx:stable-alpine@sha256:47928bf67eb62a1863cb08d9ecb52d3e704304ba614eaee17884204677702951 AS web
COPY deploy/docker/nginx.conf /etc/nginx/nginx.conf
COPY --from=frontend-build /src/web/dist/ /usr/share/nginx/html/
USER 101:101
EXPOSE 8080
# Use the immutable config directly; the stock entrypoint edits config on startup.
ENTRYPOINT ["nginx"]
CMD ["-g", "daemon off;"]
