window.SUPERBIZAGENT_CONFIG = window.SUPERBIZAGENT_CONFIG || {
    apiBaseUrl: './api',
    authToken: '',
    authTokenStorageKey: 'opscaptain-auth-token',
    siteRecord: {
        icpNumber: '湘ICP备2026013126号-1',
        icpLink: 'https://beian.miit.gov.cn/',
    },
    observability: {
        backendReadyUrl: '/ai/readyz',
        jaegerUrl: '/ai/jaeger/',
        prometheusUrl: '/ai/prometheus/',
        prometheusHealthUrl: '/ai/prometheus/-/healthy',
    },
};
