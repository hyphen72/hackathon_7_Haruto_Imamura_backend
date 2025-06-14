FROM golang:1.24 as build
WORKDIR /app
COPY . .
RUN go build -o main .
CMD ["./main"]
