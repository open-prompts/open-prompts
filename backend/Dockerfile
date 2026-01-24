# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go mod download

# Switch to a faster domestic mirror for apk
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

# Install protoc and plugins
RUN apk add protobuf protobuf-dev
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

COPY . .

# Generate Go code from proto
RUN protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    api/proto/v1/prompt.proto

RUN go build -o server cmd/server/main.go

# Run stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/server .
EXPOSE 50051 8080
CMD ["./server"]
