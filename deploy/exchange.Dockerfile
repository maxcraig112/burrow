FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /exchange ./cmd/exchange

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /exchange /exchange
ENV LOG_LEVEL=info
EXPOSE 8080
ENTRYPOINT ["/exchange"]
