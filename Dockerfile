FROM golang:1.23-alpine AS builder

WORKDIR /app

# Download dependencies first (better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
ARG VERSION=development
ARG COMMIT=unknown
ARG BUILDTIME=unknown

RUN CGO_ENABLED=0 go build \
    -trimpath \
    -pgo=auto \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILDTIME} -extldflags=-static" \
    -o /notidock

FROM alpine:latest

# Install required packages
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user and group with specific GID that matches Docker socket group
RUN addgroup -S -g 984 dockeraccess && \
    adduser -S notidock -G dockeraccess

COPY --from=builder /notidock /notidock
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER notidock

ENV GOTRACEBACK=single

HEALTHCHECK --interval=30s --timeout=3s CMD ["/notidock", "health"]

ENTRYPOINT ["/notidock"]