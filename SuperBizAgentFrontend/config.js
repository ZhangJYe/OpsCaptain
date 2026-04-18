window.SUPERBIZAGENT_CONFIG = window.SUPERBIZAGENT_CONFIG || {
    apiBaseUrl: './api',
    authToken: '',
    authTokenStorageKey: 'opscaptain-auth-token',
    observability: {
        backendReadyUrl: '/ai/readyz',
        jaegerUrl: '/ai/jaeger/',
        prometheusUrl: '/ai/prometheus/',
        prometheusHealthUrl: '/ai/prometheus/-/healthy',
    },
};
