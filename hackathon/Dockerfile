FROM golang:1.21 as build
WORKDIR /app
COPY . .
RUN go build -o main .
CMD ["./main"]
