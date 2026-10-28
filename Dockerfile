FROM golang:1.26.10-bookworm@sha256:386f0cbb8fb3402ff5a15048c96731e6ff37142d31182f3b72306359a1f5e9b7 AS backend-source
WORKDIR /src
ENV GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd/ cmd/
COPY internal/ internal/
COPY db/ db/
COPY tests/ tests/
COPY deploy/experiments/go-judge.Dockerfile deploy/experiments/go-judge.Dockerfile

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
