# Production build of the front served by nginx, which also proxies /api to the
# api-gateway: the browser sees a single origin, so no CORS is involved.
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY . .
# Real env vars win over .env.production (which points at the AWS stage).
ARG VITE_API_BASE_URL=/api/v1
ARG VITE_ENABLE_DEV_LOGIN=false
ARG VITE_USE_MOCK_API=false
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL \
    VITE_ENABLE_DEV_LOGIN=$VITE_ENABLE_DEV_LOGIN \
    VITE_USE_MOCK_API=$VITE_USE_MOCK_API
RUN npm run build

FROM nginx:1.29-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD wget -qO- http://127.0.0.1/ >/dev/null || exit 1
