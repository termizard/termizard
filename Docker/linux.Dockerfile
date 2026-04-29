FROM golang:1.21-alpine

RUN apk add --no-cache gcc musl-dev bash

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o myterm-linux ./cmd/main.go
CMD ["./myterm-linux"]
