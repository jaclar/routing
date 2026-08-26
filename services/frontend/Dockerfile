# Stage 1: Build the React application
FROM node:22-alpine AS builder

WORKDIR /app

# Install pnpm directly via npm
RUN npm install -g pnpm@latest

COPY package.json pnpm-lock.yaml* pnpm-workspace.yaml* .npmrc* ./
RUN pnpm install --dangerously-allow-all-builds

COPY . ./
RUN pnpm build

# Stage 2: Serve with Caddy
FROM caddy:2-alpine

COPY --from=builder /app/dist /usr/share/caddy
COPY Caddyfile /etc/caddy/Caddyfile

EXPOSE 80

CMD ["caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"]
