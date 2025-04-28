FROM golang:1.24 AS builder

WORKDIR /app
COPY . .
# For every Go project where we deploy to AWS EKS
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix 'static' -o api-server ./cmd

FROM gcr.io/distroless/static
COPY --from=builder /app/api-server /api-server
COPY --from=builder /app/web /web

ENTRYPOINT ["/api-server"]