# --- Stage 1: Builder ---
# Use the golang:1.21-alpine image to build our static binary
FROM golang:1.25-alpine AS builder

# We need gcc and musl-dev for CGO (required by go-sqlite3)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the Go binary
# CGO_ENABLED=1 is required for go-sqlite3
# -ldflags="-w -s" strips debug symbols, making the binary smaller
RUN CGO_ENABLED=1 go build -ldflags="-w -s -extldflags '-static'" -tags "sqlite_static" -o /app/scraper main.go

# --- Stage 2: Final Image ---
# Use the official rod image which includes a browser
FROM ghcr.io/go-rod/rod

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/scraper /app/scraper

# Create a non-root user and group with ID 1001
# This matches the runAsUser in the Kubernetes spec
RUN addgroup --gid 1001 scraper && \
    adduser --uid 1001 --gid 1001 --disabled-password scraper

# Create the /data directory for the SQLite database
# and set ownership to our new non-root user
RUN mkdir /data && \
    chown 1001:1001 /data

# Switch to the non-root user
USER 1001

# Set the entrypoint to our compiled binary
ENTRYPOINT ["/app/scraper"]
