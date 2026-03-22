# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/straumheim ./cmd/straumheim

# Stage 2: Runtime
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/straumheim /bin/straumheim

EXPOSE 8080

ENTRYPOINT ["/bin/straumheim"]
CMD ["-config", "/etc/straumheim/config.yaml"]
