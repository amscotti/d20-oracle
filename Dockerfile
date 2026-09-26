# Keep the Go version aligned with go.mod and mise.toml. TARGETOS/TARGETARCH
# are populated by Docker Buildx for cross-platform builds and left empty
# (native build) by classic builders such as Google Cloud Build — either way
# CGO_ENABLED=0 produces a static binary for the target platform.
FROM golang:1.27.3-alpine AS build

RUN apk add --no-cache ca-certificates
WORKDIR /src

COPY go.* ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/ ./web/

ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false -ldflags="-s -w" \
    -o /d20-oracle ./cmd/d20-oracle

FROM scratch

# Go's HTTPS client needs a trusted CA bundle to reach the TypeSafe API.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /d20-oracle /d20-oracle
COPY LICENSE /LICENSE

LABEL org.opencontainers.image.title="d20-oracle" \
      org.opencontainers.image.description="Roll for your D&D class — powered by TypeSafe Jev" \
      org.opencontainers.image.licenses="MIT"

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/d20-oracle"]
CMD ["-addr", ":8080"]
