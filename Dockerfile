FROM golang:alpine as builder

RUN apk update && apk add --no-cache git ca-certificates tzdata && update-ca-certificates

WORKDIR /app

# use modules
COPY go.mod .

ENV GO111MODULE=on
RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' -a \
    -o /app/teldrive .


FROM scratch

WORKDIR /app

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /app/teldrive /app/teldrive

EXPOSE 8080

ENTRYPOINT ["/app/teldrive"]