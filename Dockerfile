FROM golang:latest

WORKDIR /app

COPY . .

RUN go build -o /chatgpt-create-unit-tests

ENTRYPOINT ["/chatgpt-create-unit-tests"]
