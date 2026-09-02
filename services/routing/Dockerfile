# Stage 1: Build the Go binary
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/routing-server main.go

# Stage 2: Minimal runtime image
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/routing-server /app/routing-server

EXPOSE 4080

ENV PORT=4080
ENV VPP_SERVICE_URL=http://vpp-service:4001
ENV METEO_SERVICE_URL=http://meteo-service:4081
ENV GOMEMLIMIT=500MiB
ENV GOGC=80

CMD ["/app/routing-server"]
