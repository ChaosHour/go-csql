# syntax=docker/dockerfile:1

# ---- Builder Stage ----
FROM ubuntu:latest AS builder

# Install build dependencies and Go
ARG GO_VERSION=1.22.2 # Specify latest Go version (update as needed)
ENV PATH="/usr/local/go/bin:${PATH}"

RUN apt-get update && \
    apt-get install -y --no-install-recommends wget ca-certificates git && \
    wget "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -O /tmp/go.tar.gz && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm /tmp/go.tar.gz && \
    go version && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Copy source code and build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /csql ./cmd/csql

# ---- Final Stage ----
FROM ubuntu:latest

# Install runtime dependencies (ca-certificates for potential TLS connections)
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Copy the static binary from the builder stage
COPY --from=builder /csql /usr/local/bin/csql

# Set the entrypoint
ENTRYPOINT ["/usr/local/bin/csql"]
