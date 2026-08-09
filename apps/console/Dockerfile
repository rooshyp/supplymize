FROM node:22-alpine AS build
WORKDIR /workspace
RUN corepack enable
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY apps/console/package.json apps/console/package.json
RUN pnpm install --frozen-lockfile
COPY apps/console apps/console
RUN pnpm --filter @supplymize/console build

FROM nginxinc/nginx-unprivileged:1.27-alpine AS runtime
COPY apps/console/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /workspace/apps/console/dist /usr/share/nginx/html
EXPOSE 8080
HEALTHCHECK --interval=20s --timeout=3s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
