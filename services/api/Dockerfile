# Production image for the Go API (documents 165, 301).
#
# Two stages: the build stage carries the toolchain, the final image carries a
# static binary and nothing else. A production image with a compiler in it is a
# larger attack surface for no benefit.

FROM golang:1.25-alpine AS build

WORKDIR /src

# Dependencies first, so a code change does not re-download the module cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO off produces a static binary that runs on a distroless base.
# Build metadata is injected rather than compiled in, so the same source
# produces the same binary and the version comes from CI.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath -ldflags="-s -w" \
        -o /out/migrate ./cmd/migrate

# Distroless: no shell, no package manager, no busybox. An attacker who reaches
# code execution finds nothing to execute.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/api /api
COPY --from=build /out/migrate /migrate
# Migrations travel with the binary that applies them, so a deployed image can
# always migrate to the schema it expects.
COPY --from=build /src/migrations /migrations

# Never root.
USER nonroot:nonroot

EXPOSE 8080

# Migrations are never run at startup (documents 26, 196) — the API and the
# migrator are separate commands, so a rolling deploy cannot have two instances
# migrating at once.
ENTRYPOINT ["/api"]
