#!/usr/bin/env sh
set -eu

APP_DIR="${APP_DIR:-$(pwd)}"
COMPOSE="docker compose --env-file .env.production -f docker-compose.prod.yml"

cleanup() {
  rm -f "./${ACR_PASSWORD_FILE:-.acr-password}"
}

trap cleanup EXIT

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
auth_secret="$(sed -n 's/^AUTH_JWT_SECRET=//p' ./.env.production | tr -d '\r' | head -n 1)"

case "$auth_secret" in
  ""|"replace-with-a-32-char-secret"|"your-jwt-secret"|replace-with*|your-*)
    echo "AUTH_JWT_SECRET must be set to a strong non-placeholder value"
    exit 1
    ;;
esac

if [ "${#auth_secret}" -lt 32 ]; then
  echo "AUTH_JWT_SECRET must be at least 32 characters"
  exit 1
fi

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

$COMPOSE pull
if ! $COMPOSE up -d --wait --wait-timeout 180 --remove-orphans; then
  $COMPOSE ps || true
  $COMPOSE logs --tail=120 backend frontend caddy || true
  echo "compose deployment failed"
  exit 1
fi

attempt=0
until $COMPOSE exec -T backend wget -qO- http://127.0.0.1:8000/readyz >/dev/null; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 15 ]; then
    $COMPOSE ps || true
    $COMPOSE logs --tail=120 backend frontend caddy || true
    echo "backend readiness check failed"
    exit 1
  fi
  sleep 2
done
