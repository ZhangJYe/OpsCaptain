FROM nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY index.html /usr/share/nginx/html/index.html
COPY app.js /usr/share/nginx/html/app.js
COPY theme.css /usr/share/nginx/html/theme.css
COPY layout.css /usr/share/nginx/html/layout.css
COPY chat.css /usr/share/nginx/html/chat.css
COPY input.css /usr/share/nginx/html/input.css
COPY components.css /usr/share/nginx/html/components.css
COPY config.js /usr/share/nginx/html/config.js

EXPOSE 80
