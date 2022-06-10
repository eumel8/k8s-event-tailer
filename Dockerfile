FROM golang:1.18.3-alpine3.16

WORKDIR /go/src/github.com/sandipb/k8s-event-tailer/
COPY . .
RUN apk add --no-cache make && make

FROM alpine:3.16
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=0 /go/src/github.com/sandipb/k8s-event-tailer/k8s-event-tailer ./
ENTRYPOINT ["./k8s-event-tailer"]

# Disable stats logging by default
CMD ["--stats-interval=0"]
EXPOSE 8000  