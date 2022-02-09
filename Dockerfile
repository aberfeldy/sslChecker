FROM golang:1.16 AS build
WORKDIR /go/src/github.com/aberfeldy/sslChecker/
COPY main.go ./
COPY go.mod ./
RUN go get ./...
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine:latest AS app
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=build /go/src/github.com/aberfeldy/sslChecker/app ./
CMD ["./app"]