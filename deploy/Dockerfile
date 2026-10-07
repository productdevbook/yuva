# syntax=docker/dockerfile:1
FROM node:24-trixie-slim AS web
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/root/.npm \
    if [ -f web/package.json ]; then cd web && npm ci && npm run build; else mkdir -p web/dist; fi

FROM golang:1.27.1-trixie AS build
WORKDIR /src/api
COPY api/go.mod api/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY api/ ./
COPY --from=web /src/web/dist /tmp/web-dist
RUN if [ -f /tmp/web-dist/index.html ]; then \
      find internal/ui/dist -mindepth 1 -delete && cp -R /tmp/web-dist/. internal/ui/dist/; \
    fi
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/yuva ./cmd/yuva

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/yuva /yuva
USER 65532:65532
EXPOSE 8080 9090
ENTRYPOINT ["/yuva"]
CMD ["serve"]
