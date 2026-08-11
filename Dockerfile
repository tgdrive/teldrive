FROM oven/bun:1-alpine AS ui-build
WORKDIR /src
COPY openapi ./openapi
COPY ui/package.json ui/bun.lock ./ui/
RUN bun install --frozen-lockfile --cwd ui
COPY ui ./ui
RUN bun run --cwd ui build

FROM golang:1.26-alpine AS build
WORKDIR /src
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-build /src/ui/dist ./ui/dist
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
    -o /out/teldrive ./cmd/teldrive

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/teldrive /usr/local/bin/teldrive
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/teldrive"]
CMD ["serve"]
