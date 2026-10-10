# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:24-trixie-slim AS web
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/root/.npm \
    if [ -f web/package.json ]; then cd web && npm ci && npm run build; else mkdir -p web/dist; fi

FROM --platform=$BUILDPLATFORM oven/bun:1.4.2 AS widget
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/root/.bun/install/cache \
    mkdir -p sdk/js/dist && if [ -f sdk/js/package.json ]; then cd sdk/js && bun install --frozen-lockfile && bun run build; fi

FROM --platform=$BUILDPLATFORM golang:1.27.1-trixie AS build
WORKDIR /src/api
COPY api/go.mod api/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY api/ ./
COPY --from=web /src/web/dist /tmp/web-dist
COPY --from=widget /src/sdk/js/dist /tmp/widget-dist
RUN if [ -f /tmp/web-dist/index.html ]; then \
      find internal/ui/dist -mindepth 1 -delete && cp -R /tmp/web-dist/. internal/ui/dist/; \
    fi; \
    if [ -f /tmp/widget-dist/yuva.js ] && [ -f /tmp/widget-dist/yuva-chat.js ] && [ -f /tmp/widget-dist/yuva-docs.js ]; then \
      cp /tmp/widget-dist/yuva.js /tmp/widget-dist/yuva-chat.js /tmp/widget-dist/yuva-docs.js internal/widget/dist/; \
    fi
ARG VERSION=dev
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/yuva ./cmd/yuva
RUN mkdir -p /out/data/attachments

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/yuva /yuva
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
EXPOSE 8080 9090
ENTRYPOINT ["/yuva"]
CMD ["serve"]
