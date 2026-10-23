# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.26.8-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG DUBLIFT_SHA

RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Scarica esattamente il commit rilevato dalla GitHub Action
RUN git clone https://github.com/joojoooo/DubLift.git . \
    && git checkout "${DUBLIFT_SHA}"

# DubLift viene compilato secondo le istruzioni del progetto
RUN CGO_ENABLED=0 \
    GOOS="${TARGETOS}" \
    GOARCH="${TARGETARCH}" \
    go build -trimpath -ldflags="-s -w" \
    -o /out/dublift ./cmd/dublift


# Runtime
FROM alpine:3.22

RUN apk add --no-cache \
    ca-certificates \
    ffmpeg \
    ffmpeg-libs \
    fontconfig \
    ttf-dejavu \
    tzdata

COPY --from=builder /out/dublift /usr/local/bin/dublift

WORKDIR /data

ENTRYPOINT ["/usr/local/bin/dublift"]
