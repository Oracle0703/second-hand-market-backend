FROM golang:1.22-bookworm AS build

WORKDIR /app

COPY backend/go.mod backend/go.sum ./
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
RUN GOPROXY="$GOPROXY" GOSUMDB="$GOSUMDB" go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server

FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl libheif1 libvips-tools \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /srv

COPY --from=build /out/server /srv/server

EXPOSE 8080

CMD ["/srv/server"]
