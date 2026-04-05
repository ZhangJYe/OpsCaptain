#!/usr/bin/env sh
set -eu

APP_DIR="${APP_DIR:-$(pwd)}"

cd "$APP_DIR"

if [ ! -f "./release.env" ]; then
  echo "release.env is required"
  exit 1
fi

if [ ! -f "./.env.production" ]; then
  echo ".env.production is required"
  exit 1
fi

set -a
. ./release.env
set +a

domain_name="$(sed -n 's/^DOMAIN_NAME=//p' ./.env.production | tr -d '\r' | head -n 1)"
tls_email="$(sed -n 's/^TLS_EMAIL=//p' ./.env.production | tr -d '\r' | head -n 1)"

{
  if [ -n "$tls_email" ]; then
    echo "{"
    printf '    email %s\n' "$tls_email"
    echo "}"
    echo
  fi

  cat <<'EOF'
:80 {
    encode zstd gzip
    reverse_proxy frontend:80
}
EOF

  if [ -n "$domain_name" ]; then
    echo
    printf '%s {\n' "$domain_name"
    cat <<'EOF'
    encode zstd gzip
    reverse_proxy frontend:80
}
EOF
  fi
} > ./Caddyfile.generated

if [ -n "${ACR_PASSWORD_FILE:-}" ] && [ -f "./${ACR_PASSWORD_FILE}" ]; then
  docker login "$ACR_REGISTRY" -u "$ACR_USERNAME" --password-stdin < "./${ACR_PASSWORD_FILE}"
fi

docker compose --env-file .env.production -f docker-compose.prod.yml pull
docker compose --env-file .env.production -f docker-compose.prod.yml up -d --remove-orphans

rm -f "./${ACR_PASSWORD_FILE:-.acr-password}"

attempt=0
until docker compose --env-file .env.production -f docker-compose.prod.yml exec -T backend wget -q --spider http://127.0.0.1:8000/healthz; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 20 ]; then
    docker compose --env-file .env.production -f docker-compose.prod.yml ps || true
    docker compose --env-file .env.production -f docker-compose.prod.yml logs --tail=120 backend || true
    echo "backend health check failed"
    exit 1
  fi
  sleep 3
done
