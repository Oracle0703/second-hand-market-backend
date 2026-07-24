FROM node:22.22.2-bookworm-slim AS build

WORKDIR /app

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund

COPY frontend/ ./
ENV VITE_API_BASE_URL=/api/v1
RUN npm run build

FROM nginx:1.28.0-alpine

COPY deploy/acceptance/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80
