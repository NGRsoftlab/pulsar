FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download 

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-s -w" \
    -o /build/pulsar \
    ./cmd


FROM gcr.io/distroless/static:nonroot

COPY --from=builder /build/pulsar /app/pulsar

LABEL maintainer="nik.shabanov2018@yandex.com"
LABEL description="Pulsar: CLI tool for high EPS event generation"

ENTRYPOINT ["/app/pulsar"]
