#!/usr/bin/env sh
set -eu

APP_DIR="${APP_DIR:-$(pwd)}"
COMPOSE="docker compose --env-file .env.production -f docker-compose.prod.yml"

normalize_optional_value() {
  value="$(printf '%s' "${1:-}" | tr -d '\r' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
  case "$value" in
    ""|'${'*'}')
      printf ''
      ;;
    *)
      printf '%s' "$value"
      ;;
  esac
}

normalize_path_prefix() {
  value="$(normalize_optional_value "${1:-}")"
  case "$value" in
    "")
      printf ''
      ;;
    "/")
      printf ''
      ;;
    *)
      case "$value" in
        /*) ;;
        *) value="/$value" ;;
      esac
      value="$(printf '%s' "$value" | sed 's#/\+$##')"
      if [ "$value" = "/" ]; then
        printf ''
      else
        printf '%s' "$value"
      fi
      ;;
  esac
}

write_site_block() {
  site_label="$1"

  printf '%s {\n' "$site_label"
  cat <<EOF
    encode zstd gzip
EOF

  if [ -n "$app_base_path" ]; then
    cat <<EOF

    @siteRoot path /
    redir @siteRoot ${app_base_path}/ 308

    @appRoot path ${app_base_path}
    redir @appRoot ${app_base_path}/ 308

    @jaegerRoot path ${app_base_path}/jaeger
    redir @jaegerRoot ${app_base_path}/jaeger/ 308
    handle_path ${app_base_path}/jaeger/* {
        reverse_proxy jaeger:16686
    }
EOF

    if [ -n "$prometheus_address" ]; then
      cat <<EOF

    @prometheusRoot path ${app_base_path}/prometheus
    redir @prometheusRoot ${app_base_path}/prometheus/ 308
    handle_path ${app_base_path}/prometheus/* {
        reverse_proxy $prometheus_address
    }
EOF
    fi

    cat <<EOF

    handle_path ${app_base_path}/* {
        reverse_proxy frontend:80
    }
}
EOF
    return
  fi

  cat <<EOF

    @jaegerRoot path /jaeger
    redir @jaegerRoot /jaeger/ 308
    handle_path /jaeger/* {
        reverse_proxy jaeger:16686
    }
EOF

  if [ -n "$prometheus_address" ]; then
    cat <<EOF

    @prometheusRoot path /prometheus
    redir @prometheusRoot /prometheus/ 308
    handle_path /prometheus/* {
        reverse_proxy $prometheus_address
    }
EOF
  fi

  cat <<'EOF'

    reverse_proxy frontend:80
}
EOF
}

cleanup() {
  rm -f "./${ACR_PASSWORD_FILE:-.acr-password}"
  rm -f "${caddyfile_tmp_path:-}"
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

domain_name="$(normalize_optional_value "$(sed -n 's/^DOMAIN_NAME=//p' ./.env.production | head -n 1)")"
tls_email="$(normalize_optional_value "$(sed -n 's/^TLS_EMAIL=//p' ./.env.production | head -n 1)")"
auth_secret="$(normalize_optional_value "$(sed -n 's/^AUTH_JWT_SECRET=//p' ./.env.production | head -n 1)")"
prometheus_address="$(normalize_optional_value "$(sed -n 's/^PROMETHEUS_ADDRESS=//p' ./.env.production | head -n 1)")"
app_base_path="$(normalize_path_prefix "$(sed -n 's/^APP_BASE_PATH=//p' ./.env.production | head -n 1)")"

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

caddyfile_tmp_path="$(mktemp "${TMPDIR:-/tmp}/opscaptain-caddy.XXXXXX")"
{
  if [ -n "$tls_email" ]; then
    echo "{"
    printf '    email %s\n' "$tls_email"
    echo "}"
    echo
  fi

  write_site_block ":80"

  if [ -n "$domain_name" ]; then
    echo
    write_site_block "$domain_name"
  fi
} > "$caddyfile_tmp_path"

caddy_config_changed=1
if [ -f ./Caddyfile.generated ] && cmp -s "$caddyfile_tmp_path" ./Caddyfile.generated; then
  caddy_config_changed=0
else
  mv "$caddyfile_tmp_path" ./Caddyfile.generated
  caddyfile_tmp_path=''
fi

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

if [ "$caddy_config_changed" -eq 1 ]; then
  if ! $COMPOSE up -d --force-recreate --no-deps caddy; then
    $COMPOSE ps || true
    $COMPOSE logs --tail=120 caddy || true
    echo "caddy reload failed"
    exit 1
  fi
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
