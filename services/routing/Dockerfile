# Stage 1: Build the Go binary
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/routing-server main.go

# Stage 2: Minimal runtime image
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/routing-server /app/routing-server
COPY --from=builder /app/data /app/data

EXPOSE 8080

ENV PORT=8080
ENV VPP_SERVICE_URL=http://vpp-service:8000

CMD ["/app/routing-server"]
