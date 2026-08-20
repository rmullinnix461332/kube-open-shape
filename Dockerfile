# Build stage
FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/edge ./cmd/edge
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/kos ./cmd/kos

# Runtime stage
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /out/edge /edge
COPY --from=builder /out/kos /kos

USER nonroot:nonroot

ENTRYPOINT ["/edge"]
