FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:alpine as builder

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH


RUN apk update && apk add --no-cache git ca-certificates make tzdata nodejs npm && update-ca-certificates

WORKDIR /app


# use modules
COPY go.mod .

ENV GO111MODULE=on
RUN go mod download && go mod verify

COPY . .

#build ui
RUN make pre-ui &&  make ui

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags='-w -s -extldflags "-static"' -a \
    -o /app/teldrive cmd/teldrive/main.go


FROM --platform=${TARGETPLATFORM:-linux/amd64} busybox

WORKDIR /app

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /app/teldrive /app/teldrive

EXPOSE 8080

ENTRYPOINT ["/app/teldrive"]