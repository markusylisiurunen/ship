FROM golang:1.24 AS builder
WORKDIR /app
COPY whoami.go .
RUN CGO_ENABLED=0 go build -o whoami whoami.go

FROM alpine:3
WORKDIR /app
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/whoami .
ARG VERSION
LABEL version=$VERSION
CMD ["./whoami"]
