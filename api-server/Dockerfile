FROM golang:1.24 as builder
WORKDIR /app
COPY . .
RUN go build -o api-server ./cmd

FROM gcr.io/distroless/static
COPY --from=builder /app/api-server /api-server
ENTRYPOINT ["/api-server"]