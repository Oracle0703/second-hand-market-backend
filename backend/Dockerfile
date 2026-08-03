FROM golang:1.22-bookworm AS build

ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /app

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/migrate ./scripts/migrate
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/backfill-product-images ./scripts/backfill_product_images

FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl libheif1 libvips-tools \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /srv

COPY --from=build /out/server /srv/server
COPY --from=build /out/migrate /srv/migrate
COPY --from=build /out/backfill-product-images /srv/backfill-product-images
COPY backend/migrations /srv/migrations

ENV APP_ENV=production

EXPOSE 8080

CMD ["/srv/server"]
