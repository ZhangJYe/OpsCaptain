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

read_env_value() {
  key="$1"
  awk -F= -v key="$key" '
    $1 == key {
      sub(/^[^=]*=/, "", $0)
      value = $0
    }
    END {
      print value
    }
  ' ./.env.production
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

ensure_prometheus_bind_files() {
  if [ -d "./prometheus/prometheus.yml" ]; then
    rm -rf "./prometheus/prometheus.yml"
  fi

  if [ -d "./prometheus/opscaptionai-cost-alerts.yml" ]; then
    rm -rf "./prometheus/opscaptionai-cost-alerts.yml"
  fi

  if [ ! -f "./prometheus/prometheus.yml" ]; then
    echo "missing file: ./prometheus/prometheus.yml"
    exit 1
  fi

  if [ ! -f "./prometheus/opscaptionai-cost-alerts.yml" ]; then
    echo "missing file: ./prometheus/opscaptionai-cost-alerts.yml"
    exit 1
  fi
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
    handle ${app_base_path}/jaeger/* {
        reverse_proxy jaeger:16686
    }
EOF

    cat <<EOF

    @prometheusRoot path ${app_base_path}/prometheus
    redir @prometheusRoot ${app_base_path}/prometheus/ 308
    handle_path ${app_base_path}/prometheus/* {
        reverse_proxy $prometheus_address
    }

    @prometheusLegacy path /graph /alerts /query /rules /targets /service-discovery /status /tsdb-status /config /flags /runtimeinfo
    redir @prometheusLegacy ${app_base_path}/prometheus{uri} 308
EOF

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
    handle /jaeger/* {
        reverse_proxy jaeger:16686
    }
EOF

  cat <<EOF

    @prometheusRoot path /prometheus
    redir @prometheusRoot /prometheus/ 308
    handle_path /prometheus/* {
        reverse_proxy $prometheus_address
    }
EOF

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

domain_name="$(normalize_optional_value "$(read_env_value DOMAIN_NAME)")"
tls_email="$(normalize_optional_value "$(read_env_value TLS_EMAIL)")"
auth_secret="$(normalize_optional_value "$(read_env_value AUTH_JWT_SECRET)")"
jaeger_endpoint="$(normalize_optional_value "$(read_env_value JAEGER_ENDPOINT)")"
prometheus_address="$(normalize_optional_value "$(read_env_value PROMETHEUS_ADDRESS)")"
app_base_path="$(normalize_path_prefix "$(read_env_value APP_BASE_PATH)")"

if [ -z "$jaeger_endpoint" ]; then
  jaeger_endpoint="http://jaeger:14268/api/traces"
fi

if [ -z "$prometheus_address" ]; then
  prometheus_address="http://prometheus:9090"
fi

export JAEGER_ENDPOINT="$jaeger_endpoint"
export PROMETHEUS_ADDRESS="$prometheus_address"

if [ -n "$app_base_path" ]; then
  jaeger_base_path="${app_base_path}/jaeger"
  prometheus_external_url="${app_base_path}/prometheus/"
else
  jaeger_base_path="/jaeger"
  prometheus_external_url="/prometheus/"
fi
export JAEGER_BASE_PATH="$jaeger_base_path"
export PROMETHEUS_EXTERNAL_URL="$prometheus_external_url"

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

ensure_prometheus_bind_files

$COMPOSE pull
if ! $COMPOSE up -d --wait --wait-timeout 180 --remove-orphans; then
  $COMPOSE ps || true
  $COMPOSE logs --tail=120 backend frontend caddy jaeger prometheus || true
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
    $COMPOSE logs --tail=120 backend frontend caddy jaeger prometheus || true
    echo "backend readiness check failed"
    exit 1
  fi
  sleep 2
done
