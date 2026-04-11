window.SUPERBIZAGENT_CONFIG = window.SUPERBIZAGENT_CONFIG || {
    apiBaseUrl: '/api',
    authToken: '',
    authTokenStorageKey: 'opscaptain-auth-token',
    observability: {
        backendReadyUrl: '/readyz',
        jaegerUrl: '/jaeger/',
        prometheusUrl: '/prometheus/',
        prometheusHealthUrl: '/prometheus/-/healthy',
    },
};
