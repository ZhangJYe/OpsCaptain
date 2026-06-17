FROM node:22-alpine AS builder
WORKDIR /app
COPY package.json ./
RUN npm install
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html
COPY config.js /usr/share/nginx/html/config.js
HEALTHCHECK --interval=30s --timeout=3s CMD wget --spider http://127.0.0.1/ || exit 1
EXPOSE 80
