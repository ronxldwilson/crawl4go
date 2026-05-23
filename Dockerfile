FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS TARGETARCH
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o crawl4go ./cmd/crawl4go

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/crawl4go /usr/local/bin/crawl4go
EXPOSE 8082
ENTRYPOINT ["crawl4go"]
