FROM golang:1.24 as builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# For every Go project where we deploy to AWS EKS
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o claim-controller main.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /root/
COPY --from=builder /app/claim-controller .

ENTRYPOINT ["./claim-controller"]