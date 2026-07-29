FROM golang:1.22-bookworm AS build

WORKDIR /app

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server

FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates libheif1 libvips-tools \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /srv

COPY --from=build /out/server /srv/server

ENV APP_ENV=production

EXPOSE 8080

CMD ["/srv/server"]
