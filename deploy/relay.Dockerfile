FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /relay ./cmd/relay

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /relay /relay
ENV LOG_LEVEL=info
ENV TUNNEL_BIND=:8082
ENV TUNNEL_PUBLIC_URL=http://localhost:8082
EXPOSE 9090 8082
ENTRYPOINT ["/relay"]
