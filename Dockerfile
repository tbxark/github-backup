FROM golang:1.23 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN make build

FROM alpine:3
RUN apk add --no-cache ca-certificates git tzdata
COPY --from=builder /app/build/github-backup /main
ENTRYPOINT ["/main"]
CMD ["--config", "/config/config.json"]
