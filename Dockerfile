FROM golang:1.19 as builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o main main.go

FROM alpine:3.15.3
WORKDIR /app
COPY --from=builder /src/main .
CMD ["./main"]