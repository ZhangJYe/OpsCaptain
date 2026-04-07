window.SUPERBIZAGENT_CONFIG = window.SUPERBIZAGENT_CONFIG || {
    apiBaseUrl: '/api',
    observability: {
        backendReadyUrl: '/readyz',
        jaegerUrl: '/jaeger/',
        prometheusUrl: '/prometheus/',
        prometheusHealthUrl: '/prometheus/-/healthy',
    },
};
