# Build stage
FROM golang:alpine AS builder

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY go.mod ./
COPY main.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o dvr-stremio .

# Runtime stage - scratch for minimal image
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/dvr-stremio /dvr-stremio

ENV PORT=7001
ENV EASYPROXY_URL=http://localhost:8080
ENV EASYPROXY_PASSWORD=

EXPOSE 7001

ENTRYPOINT ["/dvr-stremio"]
