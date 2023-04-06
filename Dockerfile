FROM golang:latest

WORKDIR /app

COPY . .

RUN go mod download
RUN go build -o /chatgpt-create-unit-tests

ENTRYPOINT ["/chatgpt-create-unit-tests"]
