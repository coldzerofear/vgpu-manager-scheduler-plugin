# Build the manager binary
FROM quay.io/projectquay/golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

ENV GO111MODULE=on
ENV GOPROXY=https://goproxy.cn,direct

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY . .

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=1 CGO_CFLAGS="-D_GNU_SOURCE -D_FORTIFY_SOURCE=2 -O2 -ftrapv" \
    CGO_LDFLAGS_ALLOW='-Wl,--unresolved-symbols=ignore-in-object-files' \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o scheduler-plugin main.go

FROM quay.io/jitesoft/ubuntu:20.04

WORKDIR /
COPY --from=builder /workspace/scheduler-plugin /usr/bin/scheduler-plugin

ENTRYPOINT ["scheduler-plugin"]
