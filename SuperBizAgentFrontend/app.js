// SuperBizAgent 前端应用
class SuperBizAgentApp {
    constructor() {
        this.apiBaseUrl = this.resolveApiBaseUrl();
        this.authConfig = this.resolveAuthConfig();
        this.observability = this.resolveObservabilityConfig();
        this.currentMode = 'quick'; // 'quick' 或 'stream'
        this.sessionId = this.generateSessionId();
        this.isStreaming = false;
        this.currentChatHistory = []; // 当前对话的消息历史
        this.chatHistories = this.loadChatHistories(); // 所有历史对话
        this.isCurrentChatFromHistory = false; // 标记当前对话是否是从历史记录加载的
        this.allowedUploadExtensions = ['.md', '.txt', '.pdf', '.doc', '.docx', '.csv', '.json', '.yaml', '.yml'];
        this.maxUploadSizeMB = 20;
        this.historySearchTerm = '';
        this.abortController = null;
        
        this.initializeElements();
        this.bindEvents();
        this.updateUI();
        this.initMarkdown();
        this.checkAndSetCentered();
        this.renderChatHistory();
        this.refreshObservabilityStatus();
    }

    resolveApiBaseUrl() {
        const runtimeConfig = window.SUPERBIZAGENT_CONFIG || {};
        const configuredBase = (runtimeConfig.apiBaseUrl || '').trim();

        if (configuredBase) {
            return configuredBase.replace(/\/+$/, '');
        }

        return './api';
    }

    resolveAuthConfig() {
        const runtimeConfig = window.SUPERBIZAGENT_CONFIG || {};
        return {
            authToken: (runtimeConfig.authToken || '').trim(),
            authTokenStorageKey: (runtimeConfig.authTokenStorageKey || 'opscaptain-auth-token').trim() || 'opscaptain-auth-token',
        };
    }

    resolveAuthToken() {
        if (this.authConfig && this.authConfig.authToken) {
            return this.authConfig.authToken;
        }
        if (typeof window === 'undefined' || !window.localStorage || !this.authConfig || !this.authConfig.authTokenStorageKey) {
            return '';
        }
        return (window.localStorage.getItem(this.authConfig.authTokenStorageKey) || '').trim();
    }

    clearStoredAuthToken() {
        if (this.authConfig && this.authConfig.authToken) {
            return;
        }
        if (typeof window === 'undefined' || !window.localStorage || !this.authConfig || !this.authConfig.authTokenStorageKey) {
            return;
        }
        window.localStorage.removeItem(this.authConfig.authTokenStorageKey);
    }

    isAnonymousPublicPath(path) {
        const normalized = String(path || '').split('?')[0];
        return normalized === '/chat' || normalized === '/chat_stream';
    }

    buildApiHeaders(extraHeaders = {}, includeAuth = true) {
        const headers = { ...extraHeaders };
        if (!includeAuth) {
            return headers;
        }
        const token = this.resolveAuthToken();
        if (token) {
            headers.Authorization = `Bearer ${token}`;
        }
        return headers;
    }

    async apiFetch(path, options = {}) {
        const requestOptions = { ...options };
        requestOptions.headers = this.buildApiHeaders(options.headers || {});
        const response = await fetch(`${this.apiBaseUrl}${path}`, requestOptions);
        if (
            response.status === 401 &&
            this.isAnonymousPublicPath(path) &&
            this.resolveAuthToken()
        ) {
            this.clearStoredAuthToken();
            const retryOptions = { ...options };
            retryOptions.headers = this.buildApiHeaders(options.headers || {}, false);
            return fetch(`${this.apiBaseUrl}${path}`, retryOptions);
        }
        return response;
    }

    resolveObservabilityConfig() {
        const runtimeConfig = window.SUPERBIZAGENT_CONFIG || {};
        const configured = runtimeConfig.observability || {};

        return {
            backendReadyUrl: this.resolveObservabilityUrl(configured.backendReadyUrl, '/ai/readyz'),
            jaegerUrl: this.resolveObservabilityUrl(configured.jaegerUrl, '/ai/jaeger/'),
            prometheusUrl: this.resolveObservabilityUrl(configured.prometheusUrl, '/ai/prometheus/'),
            prometheusHealthUrl: this.resolveObservabilityUrl(configured.prometheusHealthUrl, '/ai/prometheus/-/healthy'),
        };
    }

    resolveObservabilityUrl(value, fallback) {
        const raw = String(value || '').trim();
        const target = raw || fallback;
        if (!target) {
            return '';
        }
        if (/^https?:\/\//i.test(target) || target.startsWith('/')) {
            return target;
        }
        const basePath = this.resolveFrontendBasePath();
        if (target.startsWith('./')) {
            return `${basePath}${target.slice(2)}`;
        }
        try {
            return new URL(target, window.location.href).toString();
        } catch (error) {
            return `${basePath}${target.replace(/^\/+/, '')}`;
        }
    }

    resolveFrontendBasePath() {
        const path = window.location.pathname || '/';
        if (path === '/ai' || path.startsWith('/ai/')) {
            return '/ai/';
        }
        if (path.endsWith('/')) {
            return path;
        }
        const lastSlashIndex = path.lastIndexOf('/');
        if (lastSlashIndex < 0) {
            return '/';
        }
        return path.slice(0, lastSlashIndex + 1);
    }

    // 初始化Markdown配置
    initMarkdown() {
        // 等待 marked 库加载完成
        const checkMarked = () => {
            if (typeof marked !== 'undefined') {
                try {
                    // 配置marked选项
                    marked.setOptions({
                        breaks: true,  // 支持GFM换行
                        gfm: true,     // 启用GitHub风格的Markdown
                        headerIds: false,
                        mangle: false
                    });

                    // 配置代码高亮
                    if (typeof hljs !== 'undefined') {
                        marked.setOptions({
                            highlight: function(code, lang) {
                                if (lang && hljs.getLanguage(lang)) {
                                    try {
                                        return hljs.highlight(code, { language: lang }).value;
                                    } catch (err) {
                                        console.error('代码高亮失败:', err);
                                    }
                                }
                                return code;
                            }
                        });
                    }
                    console.log('Markdown 渲染库初始化成功');
                } catch (e) {
                    console.error('Markdown 配置失败:', e);
                }
            } else {
                // 如果 marked 还没加载，等待一段时间后重试
                setTimeout(checkMarked, 100);
            }
        };
        checkMarked();
    }

    // 安全地渲染 Markdown
    renderMarkdown(content) {
        if (!content) return '';
        
        if (typeof marked === 'undefined') {
            console.warn('marked 库未加载，使用纯文本显示');
            return this.escapeHtml(content);
        }
        
        try {
            const html = marked.parse(content);
            if (typeof DOMPurify !== 'undefined') {
                return DOMPurify.sanitize(html);
            }
            return html;
        } catch (e) {
            console.error('Markdown 渲染失败:', e);
            return this.escapeHtml(content);
        }
    }

    // 高亮代码块
    highlightCodeBlocks(container) {
        if (typeof hljs !== 'undefined' && container) {
            try {
                container.querySelectorAll('pre code').forEach((block) => {
                    if (!block.classList.contains('hljs')) {
                        hljs.highlightElement(block);
                    }
                });
            } catch (e) {
                console.error('代码高亮失败:', e);
            }
        }
    }

    // 初始化DOM元素
    initializeElements() {
        // 侧边栏元素
        this.sidebar = document.querySelector('.sidebar');
        this.newChatBtn = document.getElementById('newChatBtn');
        this.aiOpsSidebarBtn = document.getElementById('aiOpsSidebarBtn');
        this.sidebarToggleBtn = document.getElementById('sidebarToggleBtn');
        this.sidebarCloseBtn = document.getElementById('sidebarCloseBtn');
        this.sidebarBackdrop = document.getElementById('sidebarBackdrop');
        this.promptCards = document.querySelectorAll('.prompt-card');
        this.historySearchInput = document.getElementById('historySearchInput');
        
        // 输入区域元素
        this.messageInput = document.getElementById('messageInput');
        this.sendButton = document.getElementById('sendButton');
        this.toolsBtn = document.getElementById('toolsBtn');
        this.toolsMenu = document.getElementById('toolsMenu');
        this.uploadFileItem = document.getElementById('uploadFileItem');
        this.modeSelectorBtn = document.getElementById('modeSelectorBtn');
        this.modeSelectorCurrent = document.getElementById('modeSelectorCurrent');
        this.modeDropdown = document.getElementById('modeDropdown');
        this.currentModeText = document.getElementById('currentModeText');
        this.fileInput = document.getElementById('fileInput');
        
        // 聊天区域元素
        this.chatMessages = document.getElementById('chatMessages');
        this.loadingOverlay = document.getElementById('loadingOverlay');
        this.chatContainer = document.querySelector('.chat-container');
        this.themeToggleBtn = document.getElementById('themeToggleBtn');
        this.themeIconMoon = document.getElementById('themeIconMoon');
        this.themeIconSun = document.getElementById('themeIconSun');

        this.applyStoredTheme();
        this.welcomeGreeting = document.getElementById('welcomeGreeting');
        this.chatHistoryList = document.getElementById('chatHistoryList');
        this.observabilityLastCheck = document.getElementById('observabilityLastCheck');
        this.refreshObservabilityBtn = document.getElementById('refreshObservabilityBtn');
        this.backendObservabilityStatus = document.getElementById('backendObservabilityStatus');
        this.backendObservabilityText = document.getElementById('backendObservabilityText');
        this.backendObservabilityLink = document.getElementById('backendObservabilityLink');
        this.jaegerObservabilityStatus = document.getElementById('jaegerObservabilityStatus');
        this.jaegerObservabilityText = document.getElementById('jaegerObservabilityText');
        this.jaegerObservabilityLink = document.getElementById('jaegerObservabilityLink');
        this.prometheusObservabilityStatus = document.getElementById('prometheusObservabilityStatus');
        this.prometheusObservabilityText = document.getElementById('prometheusObservabilityText');
        this.prometheusObservabilityLink = document.getElementById('prometheusObservabilityLink');

        if (this.backendObservabilityLink) {
            this.backendObservabilityLink.href = this.observability.backendReadyUrl;
        }
        if (this.jaegerObservabilityLink) {
            this.jaegerObservabilityLink.href = this.observability.jaegerUrl;
        }
        if (this.prometheusObservabilityLink) {
            this.prometheusObservabilityLink.href = this.observability.prometheusUrl;
        }

        if (this.fileInput) {
            this.fileInput.setAttribute('accept', this.allowedUploadExtensions.join(','));
        }
        
        // 初始化时检查是否需要居中
        this.checkAndSetCentered();
    }

    // 绑定事件监听器
    bindEvents() {
        // 新建对话
        if (this.newChatBtn) {
            this.newChatBtn.addEventListener('click', () => this.newChat());
        }
        
        // AI Ops按钮
        if (this.aiOpsSidebarBtn) {
            this.aiOpsSidebarBtn.addEventListener('click', () => this.triggerAIOps());
        }

        if (this.sidebarToggleBtn) {
            this.sidebarToggleBtn.addEventListener('click', () => this.toggleSidebar());
        }

        if (this.sidebarCloseBtn) {
            this.sidebarCloseBtn.addEventListener('click', () => this.closeSidebar());
        }

        if (this.sidebarBackdrop) {
            this.sidebarBackdrop.addEventListener('click', () => this.closeSidebar());
        }

        if (this.themeToggleBtn) {
            this.themeToggleBtn.addEventListener('click', () => this.toggleTheme());
        }

        if (this.refreshObservabilityBtn) {
            this.refreshObservabilityBtn.addEventListener('click', () => this.refreshObservabilityStatus(true));
        }
        this.bindObservabilityLinkActions();

        if (this.promptCards) {
            this.promptCards.forEach((card) => {
                card.addEventListener('click', () => {
                    const prompt = card.getAttribute('data-prompt') || '';
                    this.applyPrompt(prompt);
                });
            });
        }

        if (this.historySearchInput) {
            this.historySearchInput.addEventListener('input', (e) => {
                this.historySearchTerm = e.target.value.trim().toLowerCase();
                this.renderChatHistory();
            });
        }
        
        // 模式选择下拉菜单
        if (this.modeSelectorBtn) {
            this.modeSelectorBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                this.toggleModeDropdown();
            });
        }
        
        // 下拉菜单项点击
        const dropdownItems = document.querySelectorAll('.dropdown-item');
        dropdownItems.forEach(item => {
            item.addEventListener('click', (e) => {
                const mode = item.getAttribute('data-mode');
                this.selectMode(mode);
                this.closeModeDropdown();
            });
        });
        
        // 点击外部关闭下拉菜单
        document.addEventListener('click', (e) => {
            if (!this.modeSelectorBtn || !this.modeDropdown) {
                return;
            }
            if (!this.modeSelectorBtn.contains(e.target) &&
                !this.modeDropdown.contains(e.target)) {
                this.closeModeDropdown();
            }
        });
        
        // 发送消息
        if (this.sendButton) {
            this.sendButton.addEventListener('click', () => {
                if (this.isStreaming) {
                    this.stopCurrentRequest();
                    return;
                }
                this.sendMessage();
            });
        }
        
        if (this.messageInput) {
            this.messageInput.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    this.sendMessage();
                }
            });
            this.messageInput.addEventListener('input', () => this.autoResizeInput());
        }
        
        // 工具按钮和菜单
        if (this.toolsBtn) {
            this.toolsBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                this.toggleToolsMenu();
            });
        }
        
        // 工具菜单项点击事件
        if (this.uploadFileItem) {
            this.uploadFileItem.addEventListener('click', () => {
                if (this.fileInput) {
                    this.fileInput.click();
                }
                this.closeToolsMenu();
            });
        }
        
        // 点击外部关闭工具菜单
        document.addEventListener('click', (e) => {
            if (this.toolsBtn && this.toolsMenu && 
                !this.toolsBtn.contains(e.target) && 
                !this.toolsMenu.contains(e.target)) {
                this.closeToolsMenu();
            }
        });
        
        if (this.fileInput) {
            this.fileInput.addEventListener('change', (e) => this.handleFileSelect(e));
        }
    }

    toggleSidebar(forceOpen) {
        const nextState = typeof forceOpen === 'boolean' ? forceOpen : !document.body.classList.contains('sidebar-open');
        document.body.classList.toggle('sidebar-open', nextState);
    }

    closeSidebar() {
        this.toggleSidebar(false);
    }

    async probeObservabilityEndpoint(url, options = {}) {
        const requiredAll = Array.isArray(options.requiredAll) ? options.requiredAll : [];
        const forbiddenAny = Array.isArray(options.forbiddenAny) ? options.forbiddenAny : [];
        const expectedContentType = String(options.expectedContentType || '').toLowerCase();
        try {
            const response = await fetch(url, {
                method: 'GET',
                cache: 'no-store',
                headers: {
                    'Cache-Control': 'no-cache',
                },
            });
            const contentType = String(response.headers.get('content-type') || '').toLowerCase();
            let bodyText = '';
            if (requiredAll.length > 0 || forbiddenAny.length > 0 || expectedContentType) {
                bodyText = (await response.text()).toLowerCase();
            }
            let ok = response.ok;
            if (ok && expectedContentType) {
                ok = contentType.includes(expectedContentType);
            }
            if (ok && requiredAll.length > 0) {
                ok = requiredAll.every((keyword) => bodyText.includes(String(keyword).toLowerCase()));
            }
            if (ok && forbiddenAny.length > 0) {
                ok = !forbiddenAny.some((keyword) => bodyText.includes(String(keyword).toLowerCase()));
            }
            return {
                ok,
                status: response.status,
                contentType,
            };
        } catch (error) {
            return {
                ok: false,
                error: error.message || 'network error',
            };
        }
    }

    updateObservabilityCard(statusElement, textElement, linkElement, state) {
        if (!statusElement || !textElement || !linkElement) {
            return;
        }

        statusElement.className = `observability-status observability-status-${state.variant}`;
        statusElement.textContent = state.label;
        textElement.textContent = state.text;

        if (state.disabled) {
            linkElement.classList.add('is-disabled');
            linkElement.setAttribute('aria-disabled', 'true');
        } else {
            linkElement.classList.remove('is-disabled');
            linkElement.removeAttribute('aria-disabled');
        }
    }

    async refreshObservabilityStatus(manual = false) {
        if (this.refreshObservabilityBtn) {
            this.refreshObservabilityBtn.disabled = true;
            this.refreshObservabilityBtn.textContent = 'Checking...';
        }

        const now = new Date();
        if (this.observabilityLastCheck) {
            this.observabilityLastCheck.textContent = `Last check ${now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`;
        }

        const [backend, jaeger, prometheus] = await Promise.all([
            this.probeObservabilityEndpoint(this.observability.backendReadyUrl),
            this.probeObservabilityEndpoint(this.observability.jaegerUrl, {
                expectedContentType: 'text/html',
                requiredAll: ['jaeger ui'],
                forbiddenAny: ['opscaption'],
            }),
            this.probeObservabilityEndpoint(this.observability.prometheusHealthUrl, {
                expectedContentType: 'text/plain',
                requiredAll: ['prometheus', 'healthy'],
                forbiddenAny: ['opscaption'],
            }),
        ]);

        this.updateObservabilityCard(
            this.backendObservabilityStatus,
            this.backendObservabilityText,
            this.backendObservabilityLink,
            backend.ok
                ? { variant: 'live', label: 'Live', text: 'Backend readiness endpoint is responding normally.' }
                : { variant: 'down', label: 'Down', text: `Backend readiness probe failed${backend.status ? ` (HTTP ${backend.status})` : ''}.` }
        );

        this.updateObservabilityCard(
            this.jaegerObservabilityStatus,
            this.jaegerObservabilityText,
            this.jaegerObservabilityLink,
            jaeger.ok
                ? { variant: 'live', label: 'Live', text: 'Jaeger UI is reachable from the current deployment.' }
                : { variant: 'down', label: 'Down', text: `Jaeger UI is not reachable${jaeger.status ? ` (HTTP ${jaeger.status})` : ''}.` }
        );

        this.updateObservabilityCard(
            this.prometheusObservabilityStatus,
            this.prometheusObservabilityText,
            this.prometheusObservabilityLink,
            prometheus.ok
                ? { variant: 'live', label: 'Live', text: 'Prometheus health endpoint is reachable from the current deployment.' }
                : { variant: 'down', label: 'Down', text: `Prometheus endpoint is not reachable${prometheus.status ? ` (HTTP ${prometheus.status})` : ''}.`, disabled: !manual && !prometheus.status }
        );

        if (this.refreshObservabilityBtn) {
            this.refreshObservabilityBtn.disabled = false;
            this.refreshObservabilityBtn.textContent = 'Refresh';
        }
    }

    bindObservabilityLinkActions() {
        const links = [
            this.backendObservabilityLink,
            this.jaegerObservabilityLink,
            this.prometheusObservabilityLink,
        ];
        links.forEach((link) => {
            if (!link) {
                return;
            }
            link.addEventListener('click', (event) => {
                if (link.classList.contains('is-disabled')) {
                    event.preventDefault();
                    return;
                }
                const targetUrl = (link.href || '').trim();
                if (!targetUrl) {
                    return;
                }
                event.preventDefault();
                const opened = window.open(targetUrl, '_blank', 'noopener,noreferrer');
                if (opened) {
                    try {
                        opened.opener = null;
                    } catch (error) {
                        // ignore cross-origin opener assignment failures
                    }
                    return;
                }
                window.location.assign(targetUrl);
            });
        });
    }

    applyPrompt(prompt) {
        if (!prompt || !this.messageInput) {
            return;
        }
        this.messageInput.value = prompt;
        this.autoResizeInput();
        this.closeSidebar();
        this.messageInput.focus();
        this.sendMessage();
    }

    autoResizeInput() {
        if (!this.messageInput || this.messageInput.tagName !== 'TEXTAREA') {
            return;
        }
        this.messageInput.style.height = 'auto';
        this.messageInput.style.height = `${Math.min(this.messageInput.scrollHeight, 180)}px`;
    }

    stopCurrentRequest() {
        if (!this.abortController) {
            return;
        }
        this.abortController.abort();
        this.abortController = null;
        this.isStreaming = false;
        this.updateUI();
        this.showNotification('已停止当前生成', 'info');
    }

    createAbortController() {
        this.abortController = new AbortController();
        return this.abortController;
    }

    clearAbortController() {
        this.abortController = null;
    }

    isAbortError(error) {
        return error && (error.name === 'AbortError' || String(error.message || '').toLowerCase().includes('abort'));
    }

    async copyToClipboard(text) {
        if (!text) {
            return;
        }
        try {
            if (navigator.clipboard && navigator.clipboard.writeText) {
                await navigator.clipboard.writeText(text);
            } else {
                const textarea = document.createElement('textarea');
                textarea.value = text;
                textarea.style.position = 'fixed';
                textarea.style.opacity = '0';
                document.body.appendChild(textarea);
                textarea.select();
                document.execCommand('copy');
                document.body.removeChild(textarea);
            }
            this.showNotification('内容已复制', 'success');
        } catch (error) {
            this.showNotification('复制失败，请手动复制', 'error');
        }
    }

    appendMessageActions(messageContentWrapper, type, content) {
        if (!messageContentWrapper || type !== 'assistant' || !content) {
            return;
        }

        const existingActions = messageContentWrapper.querySelector('.message-actions');
        if (existingActions) {
            existingActions.remove();
        }

        const actions = document.createElement('div');
        actions.className = 'message-actions';

        const copyButton = document.createElement('button');
        copyButton.type = 'button';
        copyButton.className = 'message-action-btn';
        copyButton.innerHTML = `
            <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                <rect x="9" y="9" width="10" height="10" rx="2" stroke="currentColor" stroke-width="2"/>
                <path d="M5 15V7C5 5.89543 5.89543 5 7 5H15" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
            </svg>
            <span>复制</span>
        `;
        copyButton.addEventListener('click', () => this.copyToClipboard(content));

        actions.appendChild(copyButton);
        messageContentWrapper.appendChild(actions);
    }

    // 切换工具菜单显示/隐藏
    toggleToolsMenu() {
        if (this.toolsMenu && this.toolsBtn) {
            const wrapper = this.toolsBtn.closest('.tools-btn-wrapper');
            if (wrapper) {
                wrapper.classList.toggle('active');
            }
        }
    }

    // 关闭工具菜单
    closeToolsMenu() {
        if (this.toolsMenu && this.toolsBtn) {
            const wrapper = this.toolsBtn.closest('.tools-btn-wrapper');
            if (wrapper) {
                wrapper.classList.remove('active');
            }
        }
    }

    // 新建对话
    newChat() {
        if (this.isStreaming) {
            this.showNotification('请等待当前对话完成后再新建对话', 'warning');
            return;
        }
        
        // 如果当前有对话内容，且不是从历史记录加载的，才保存为新的历史对话
        // 如果是从历史记录加载的，只需要更新该历史记录
        if (this.currentChatHistory.length > 0) {
            if (this.isCurrentChatFromHistory) {
                // 当前对话是从历史记录加载的，更新该历史记录
                this.updateCurrentChatHistory();
            } else {
                // 当前对话是新对话，保存为新的历史对话
                this.saveCurrentChat();
            }
        }
        
        // 停止所有进行中的操作
        this.isStreaming = false;
        
        // 清空输入框
        if (this.messageInput) {
            this.messageInput.value = '';
        }
        
        // 清空当前对话历史
        this.currentChatHistory = [];
        
        // 重置标记
        this.isCurrentChatFromHistory = false;
        
        // 清空聊天记录
        if (this.chatMessages) {
            this.chatMessages.innerHTML = '';
        }
        
        // 生成新的会话ID
        this.sessionId = this.generateSessionId();
        
        // 重置模式为快速
        this.currentMode = 'quick';
        this.updateUI();
        
        // 重新设置居中样式（确保对话框居中显示）
        this.checkAndSetCentered();
        
        // 确保容器有过渡动画
        if (this.chatContainer) {
            this.chatContainer.style.transition = 'all 0.5s ease';
        }
        
        // 更新历史对话列表
        this.renderChatHistory();
        this.closeSidebar();
    }
    
    // 保存当前对话到历史记录（新建）
    saveCurrentChat() {
        if (this.currentChatHistory.length === 0) {
            return;
        }
        
        // 检查是否已存在相同ID的历史记录
        const existingIndex = this.chatHistories.findIndex(h => h.id === this.sessionId);
        if (existingIndex !== -1) {
            // 如果已存在，更新而不是新建
            this.updateCurrentChatHistory();
            return;
        }
        
        // 获取对话标题（使用第一条用户消息的前30个字符）
        const firstUserMessage = this.currentChatHistory.find(msg => msg.type === 'user');
        const title = firstUserMessage ? 
            (firstUserMessage.content.substring(0, 30) + (firstUserMessage.content.length > 30 ? '...' : '')) : 
            '新对话';
        
        const chatHistory = {
            id: this.sessionId,
            title: title,
            messages: [...this.currentChatHistory],
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString()
        };
        
        // 添加到历史记录列表的开头
        this.chatHistories.unshift(chatHistory);
        
        // 限制历史记录数量（最多保存50条）
        if (this.chatHistories.length > 50) {
            this.chatHistories = this.chatHistories.slice(0, 50);
        }
        
        // 保存到localStorage
        this.saveChatHistories();
    }
    
    // 更新当前对话的历史记录
    updateCurrentChatHistory() {
        if (this.currentChatHistory.length === 0) {
            return;
        }
        
        const existingIndex = this.chatHistories.findIndex(h => h.id === this.sessionId);
        if (existingIndex === -1) {
            // 如果不存在，调用保存方法
            this.saveCurrentChat();
            return;
        }
        
        // 更新现有的历史记录
        const history = this.chatHistories[existingIndex];
        history.messages = [...this.currentChatHistory];
        history.updatedAt = new Date().toISOString();
        
        // 如果标题需要更新（第一条消息改变了）
        const firstUserMessage = this.currentChatHistory.find(msg => msg.type === 'user');
        if (firstUserMessage) {
            const newTitle = firstUserMessage.content.substring(0, 30) + (firstUserMessage.content.length > 30 ? '...' : '');
            if (history.title !== newTitle) {
                history.title = newTitle;
            }
        }
        
        // 保存到localStorage
        this.saveChatHistories();
    }
    
    // 加载历史对话列表
    loadChatHistories() {
        try {
            const stored = localStorage.getItem('chatHistories');
            return stored ? JSON.parse(stored) : [];
        } catch (e) {
            console.error('加载历史对话失败:', e);
            return [];
        }
    }
    
    // 保存历史对话列表到localStorage
    saveChatHistories() {
        try {
            localStorage.setItem('chatHistories', JSON.stringify(this.chatHistories));
        } catch (e) {
            console.error('保存历史对话失败:', e);
        }
    }
    
    // 渲染历史对话列表
    renderChatHistory() {
        if (!this.chatHistoryList) {
            return;
        }
        
        this.chatHistoryList.innerHTML = '';
        
        const filteredHistories = this.chatHistories.filter((history) => {
            if (!this.historySearchTerm) {
                return true;
            }
            const searchableText = [
                history.title,
                ...(history.messages || []).map((message) => message.content || '')
            ].join(' ').toLowerCase();
            return searchableText.includes(this.historySearchTerm);
        });

        if (filteredHistories.length === 0) {
            const empty = document.createElement('div');
            empty.className = 'history-empty';
            empty.textContent = this.historySearchTerm ? '没有匹配的历史会话' : '还没有保存的会话';
            this.chatHistoryList.appendChild(empty);
            return;
        }
        
        filteredHistories.forEach((history, index) => {
            const historyItem = document.createElement('div');
            historyItem.className = 'history-item';
            if (history.id === this.sessionId) {
                historyItem.classList.add('active');
            }
            historyItem.dataset.historyId = history.id;
            
            historyItem.innerHTML = `
                <div class="history-item-content">
                    <span class="history-item-title">${this.escapeHtml(history.title)}</span>
                </div>
                <button class="history-item-delete" data-history-id="${history.id}" title="删除">
                    <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                        <path d="M18 6L6 18M6 6L18 18" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
                    </svg>
                </button>
            `;
            
            // 点击历史项加载对话
            historyItem.addEventListener('click', (e) => {
                if (!e.target.closest('.history-item-delete')) {
                    this.loadChatHistory(history.id);
                }
            });
            
            // 删除历史对话
            const deleteBtn = historyItem.querySelector('.history-item-delete');
            deleteBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                this.deleteChatHistory(history.id);
            });
            
            this.chatHistoryList.appendChild(historyItem);
        });
    }
    
    // 加载历史对话
    loadChatHistory(historyId) {
        const history = this.chatHistories.find(h => h.id === historyId);
        if (!history) {
            return;
        }
        
        // 如果当前有对话内容，且不是同一个对话，先保存
        if (this.currentChatHistory.length > 0 && this.sessionId !== historyId) {
            if (this.isCurrentChatFromHistory) {
                // 如果当前对话也是从历史记录加载的，更新它
                this.updateCurrentChatHistory();
            } else {
                // 如果当前对话是新对话，保存为新历史
                this.saveCurrentChat();
            }
        }
        
        // 加载历史对话
        this.sessionId = history.id;
        this.currentChatHistory = [...history.messages];
        this.isCurrentChatFromHistory = true; // 标记为从历史记录加载
        
        // 清空并重新渲染消息
        if (this.chatMessages) {
            this.chatMessages.innerHTML = '';
            history.messages.forEach(msg => {
                if (msg.type === 'assistant' && msg.meta && msg.meta.mode) {
                    this.addAssistantMessageWithMeta(msg.content, msg.meta, false);
                } else {
                    this.addMessage(msg.type, msg.content, false, false); // false表示不是流式，false表示不保存到历史（因为已经存在）
                }
            });
        }
        
        // 更新UI
        this.checkAndSetCentered();
        this.renderChatHistory();
        this.closeSidebar();
    }
    
    // 删除历史对话
    deleteChatHistory(historyId) {
        this.chatHistories = this.chatHistories.filter(h => h.id !== historyId);
        this.saveChatHistories();
        this.renderChatHistory();
        
        // 如果删除的是当前对话，清空当前对话
        if (this.sessionId === historyId) {
            this.currentChatHistory = [];
            if (this.chatMessages) {
                this.chatMessages.innerHTML = '';
            }
            this.sessionId = this.generateSessionId();
            this.checkAndSetCentered();
        }
    }

    // 切换模式下拉菜单
    toggleModeDropdown() {
        if (this.modeSelectorBtn && this.modeDropdown) {
            const wrapper = this.modeSelectorBtn.closest('.mode-selector-wrapper');
            if (wrapper) {
                wrapper.classList.toggle('active');
            }
        }
    }

    // 关闭模式下拉菜单
    closeModeDropdown() {
        if (this.modeSelectorBtn && this.modeDropdown) {
            const wrapper = this.modeSelectorBtn.closest('.mode-selector-wrapper');
            if (wrapper) {
                wrapper.classList.remove('active');
            }
        }
    }

    // 选择模式
    selectMode(mode) {
        if (this.isStreaming) {
            this.showNotification('请等待当前对话完成后再切换模式', 'warning');
            return;
        }
        
        this.currentMode = mode;
        this.updateUI();
        
        const modeNames = {
            'quick': '快速',
            'stream': '流式'
        };
        
        this.showNotification(`已切换到${modeNames[mode]}模式`, 'info');
    }

    // 更新UI
    updateUI() {
        // 更新模式选择器显示
        if (this.currentModeText) {
            const modeNames = {
                'quick': '快速',
                'stream': '流式'
            };
            this.currentModeText.textContent = modeNames[this.currentMode] || '快速';
        }

        if (this.modeSelectorCurrent) {
            const selectorNames = {
                'quick': '快速回答',
                'stream': '流式回答'
            };
            this.modeSelectorCurrent.textContent = selectorNames[this.currentMode] || '快速回答';
        }
        
        // 更新下拉菜单选中状态
        const dropdownItems = document.querySelectorAll('.dropdown-item');
        dropdownItems.forEach(item => {
            const mode = item.getAttribute('data-mode');
            if (mode === this.currentMode) {
                item.classList.add('active');
            } else {
                item.classList.remove('active');
            }
        });
        
        // 更新发送按钮状态
        if (this.sendButton) {
            this.sendButton.disabled = false;
            this.sendButton.classList.toggle('stop-mode', this.isStreaming);
            this.sendButton.title = this.isStreaming ? '停止生成' : '发送';
            this.sendButton.innerHTML = this.isStreaming ? `
                <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                    <rect x="7" y="7" width="10" height="10" rx="2" fill="currentColor"/>
                </svg>
            ` : `
                <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                    <path d="M22 2L11 13M22 2L15 22L11 13M22 2L2 9L11 13" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
            `;
        }
        
        // 更新输入框状态
        if (this.messageInput) {
            this.messageInput.disabled = this.isStreaming;
            this.messageInput.placeholder = this.currentMode === 'stream'
                ? '继续提问，实时查看生成过程'
                : '有问题，尽管问';
        }
    }

    // 生成随机会话ID
    generateSessionId() {
        return 'session_' + Math.random().toString(36).substr(2, 9) + '_' + Date.now();
    }

    // 发送消息
    async sendMessage() {
        let message = '';
        if (this.messageInput) {
            message = this.messageInput.value.trim();
        }
        
        if (!message) {
            this.showNotification('请输入消息内容', 'warning');
            return;
        }

        if (this.isStreaming) {
            this.showNotification('请等待当前对话完成', 'warning');
            return;
        }

        // 显示用户消息
        this.addMessage('user', message);
        
        // 清空输入框
        if (this.messageInput) {
            this.messageInput.value = '';
            this.autoResizeInput();
        }

        // 设置发送状态
        this.isStreaming = true;
        this.createAbortController();
        this.updateUI();

        this.addThinkingBubble();

        try {
            if (this.currentMode === 'quick') {
                await this.sendQuickMessage(message);
            } else if (this.currentMode === 'stream') {
                await this.sendStreamMessage(message);
            }
        } catch (error) {
            this.removeThinkingBubble();
            if (!this.isAbortError(error)) {
                console.error('发送消息失败:', error);
                this.addMessage('assistant', '抱歉，发送消息时出现错误：' + error.message);
            }
        } finally {
            this.isStreaming = false;
            this.clearAbortController();
            this.updateUI();
            
            // 如果当前对话是从历史记录加载的，更新历史记录
            if (this.isCurrentChatFromHistory && this.currentChatHistory.length > 0) {
                this.updateCurrentChatHistory();
                this.renderChatHistory(); // 更新历史对话列表显示
            }
        }
    }

    // 发送快速消息（普通对话）
    async sendQuickMessage(message) {
        try {
            const response = await this.apiFetch('/chat', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                signal: this.abortController ? this.abortController.signal : undefined,
                body: JSON.stringify({
                    Id: this.sessionId,
                    Question: message
                })
            });

            if (!response.ok) {
                throw new Error(`HTTP错误: ${response.status}`);
            }

            const data = await response.json();
            
            this.removeThinkingBubble();

            if (data.message === 'OK' && data.data && data.data.answer) {
                this.addAssistantMessageWithMeta(data.data.answer, {
                    mode: data.data.mode || 'legacy',
                    traceId: data.data.trace_id || '',
                    details: data.data.detail || [],
                    cached: data.data.cached || false,
                    degraded: data.data.degraded || false,
                    degradationReason: data.data.degradation_reason || ''
                });
            } else {
                throw new Error(data.message || '未知错误');
            }
        } catch (error) {
            throw error;
        }
    }

    // 发送流式消息
    async sendStreamMessage(message) {
        try {
            const response = await this.apiFetch('/chat_stream', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                signal: this.abortController ? this.abortController.signal : undefined,
                body: JSON.stringify({
                    Id: this.sessionId,
                    Question: message
                })
            });

            if (!response.ok) {
                throw new Error(`HTTP错误: ${response.status}`);
            }

            const contentType = String(response.headers.get('content-type') || '').toLowerCase();
            if (!contentType.includes('text/event-stream') || !response.body) {
                return this.sendQuickMessage(message);
            }
            
            const assistantMessageElement = this.addMessage('assistant', '', true);
            this.removeThinkingBubble();
            let fullResponse = '';
            let isFinalized = false;
            let responseMeta = { mode: '', traceId: '', details: [] };
            let streamError = '';
            let switchedToQuickFallback = false;

            const finalizeStream = () => {
                if (isFinalized) {
                    return;
                }
                isFinalized = true;
                const finalResponse = fullResponse.trim() || (streamError ? `流式输出中断：${streamError}` : '未收到可展示的流式文本输出。');
                if (assistantMessageElement) {
                    assistantMessageElement.classList.remove('streaming');
                    const messageContent = assistantMessageElement.querySelector('.message-content');
                    const messageContentWrapper = assistantMessageElement.querySelector('.message-content-wrapper');
                    if (messageContent) {
                        messageContent.innerHTML = this.renderMarkdown(finalResponse);
                        this.highlightCodeBlocks(messageContent);
                        this.renderAssistantMeta(assistantMessageElement, messageContentWrapper, responseMeta);
                        this.renderAssistantDetails(
                            assistantMessageElement,
                            messageContentWrapper,
                            responseMeta.details || [],
                            responseMeta.mode === 'aiops' ? '查看详细步骤' : '查看执行步骤'
                        );
                        this.appendMessageActions(messageContentWrapper, 'assistant', finalResponse);
                    }
                }
                if (finalResponse) {
                    this.persistAssistantHistory(finalResponse, responseMeta);
                    if (this.isCurrentChatFromHistory) {
                        this.updateCurrentChatHistory();
                        this.renderChatHistory();
                    }
                }
            };

            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let buffer = '';

            const parseSseBlock = (rawBlock) => {
                const block = String(rawBlock || '').replace(/\r/g, '');
                const lines = block.split('\n');
                let eventName = '';
                const dataLines = [];
                for (const rawLine of lines) {
                    const line = rawLine || '';
                    if (!line) {
                        continue;
                    }
                    if (line.startsWith(':')) {
                        continue;
                    }
                    if (line.startsWith('event:')) {
                        eventName = line.slice(6).trim();
                        continue;
                    }
                    if (line.startsWith('data:')) {
                        dataLines.push(line.slice(5).replace(/^\s/, ''));
                    }
                }
                return {
                    eventName: eventName || 'message',
                    data: dataLines.join('\n')
                };
            };

            const applySseEvent = async (eventName, data) => {
                if (eventName === 'connected') {
                    return false;
                }
                if (eventName === 'meta') {
                    try {
                        responseMeta = this.normalizeAssistantMeta(JSON.parse(data));
                    } catch (error) {
                        console.warn('解析stream meta失败:', error);
                    }
                    return false;
                }
                if (eventName === 'message') {
                    if (data === '') {
                        fullResponse += '\n';
                    } else {
                        fullResponse += data;
                    }
                    if (assistantMessageElement) {
                        const messageContent = assistantMessageElement.querySelector('.message-content');
                        if (messageContent) {
                            messageContent.textContent = fullResponse;
                            this.scrollToBottom();
                        }
                    }
                    return false;
                }
                if (eventName === 'error') {
                    streamError = data || 'stream error';
                    return true;
                }
                if (eventName === 'done' || data === '[DONE]') {
                    finalizeStream();
                    return true;
                }
                return false;
            };

            const switchToQuickFallback = async () => {
                if (switchedToQuickFallback) {
                    return;
                }
                switchedToQuickFallback = true;
                if (assistantMessageElement && assistantMessageElement.parentNode) {
                    assistantMessageElement.parentNode.removeChild(assistantMessageElement);
                }
                this.showNotification('流式输出中断，已自动切换到快速回答', 'warning');
                await this.sendQuickMessage(message);
            };

            try {
                let shouldStop = false;
                while (true) {
                    const { done, value } = await reader.read();
                    
                    if (done) {
                        if (buffer.trim()) {
                            const parsed = parseSseBlock(buffer);
                            shouldStop = await applySseEvent(parsed.eventName, parsed.data);
                        }
                        if (streamError && !fullResponse.trim()) {
                            await switchToQuickFallback();
                        } else if (!switchedToQuickFallback) {
                            finalizeStream();
                        }
                        break;
                    }

                    buffer += decoder.decode(value, { stream: true });

                    while (true) {
                        const match = buffer.match(/\r?\n\r?\n/);
                        if (!match || match.index === undefined) {
                            break;
                        }
                        const splitIndex = match.index;
                        const block = buffer.slice(0, splitIndex);
                        buffer = buffer.slice(splitIndex + match[0].length);
                        if (!block.trim()) {
                            continue;
                        }
                        const parsed = parseSseBlock(block);
                        shouldStop = await applySseEvent(parsed.eventName, parsed.data);
                        if (shouldStop) {
                            break;
                        }
                    }
                    if (shouldStop) {
                        if (streamError && !fullResponse.trim()) {
                            await switchToQuickFallback();
                        } else if (!switchedToQuickFallback) {
                            finalizeStream();
                        }
                        break;
                    }
                }
            } catch (error) {
                if (this.isAbortError(error)) {
                    finalizeStream();
                    return;
                }
                if (streamError && !fullResponse.trim()) {
                    await switchToQuickFallback();
                    return;
                }
                throw error;
            } finally {
                reader.releaseLock();
            }
        } catch (error) {
            throw error;
        }
    }

    // 添加消息到聊天界面
    addMessage(type, content, isStreaming = false, saveToHistory = true) {
        // 检查是否是第一条消息，如果是则移除居中样式
        const isFirstMessage = this.chatMessages && this.chatMessages.querySelectorAll('.message').length === 0;
        
        // 保存消息到当前对话历史（如果不是流式消息且需要保存）
        if (!isStreaming && saveToHistory && content) {
            this.currentChatHistory.push({
                type: type,
                content: content,
                timestamp: new Date().toISOString()
            });
        }
        
        const messageDiv = document.createElement('div');
        messageDiv.className = `message ${type}${isStreaming ? ' streaming' : ''}`;

        // 如果是assistant消息，添加头像图标
        if (type === 'assistant') {
            const messageAvatar = document.createElement('div');
            messageAvatar.className = 'message-avatar';
            messageAvatar.innerHTML = `
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                    <path d="M12 2L15.09 8.26L22 9.27L17 14.14L18.18 21.02L12 17.77L5.82 21.02L7 14.14L2 9.27L8.91 8.26L12 2Z" fill="white"/>
                </svg>
            `;
            messageDiv.appendChild(messageAvatar);
        }

        // 创建消息内容包装器
        const messageContentWrapper = document.createElement('div');
        messageContentWrapper.className = 'message-content-wrapper';

        const messageContent = document.createElement('div');
        messageContent.className = 'message-content';
        
        // 如果是assistant消息且不是流式消息，使用Markdown渲染
        if (type === 'assistant' && !isStreaming) {
            messageContent.innerHTML = this.renderMarkdown(content);
            // 高亮代码块
            this.highlightCodeBlocks(messageContent);
        } else {
            // 用户消息或流式消息使用纯文本
            messageContent.textContent = content;
        }

        messageContentWrapper.appendChild(messageContent);
        this.appendMessageActions(messageContentWrapper, type, content);
        messageDiv.appendChild(messageContentWrapper);

        if (this.chatMessages) {
            this.chatMessages.appendChild(messageDiv);
            
            // 如果是第一条消息，移除居中样式并添加动画
            if (isFirstMessage && this.chatContainer) {
                this.chatContainer.classList.remove('centered');
                // 添加动画类
                this.chatContainer.style.transition = 'all 0.5s ease';
            }
            
            this.scrollToBottom();
        }

        return messageDiv;
    }

    normalizeAssistantMeta(meta = {}) {
        const details = meta.details || meta.detail || [];
        return {
            mode: meta.mode || '',
            traceId: meta.traceId || meta.trace_id || '',
            details: Array.isArray(details) ? details : [],
            cached: Boolean(meta.cached),
            degraded: Boolean(meta.degraded),
            degradationReason: meta.degradationReason || meta.degradation_reason || '',
            approvalRequired: Boolean(meta.approvalRequired || meta.approval_required),
            approvalRequestId: meta.approvalRequestId || meta.approval_request_id || '',
            approvalStatus: meta.approvalStatus || meta.approval_status || '',
            executionPlan: Array.isArray(meta.executionPlan || meta.execution_plan)
                ? (meta.executionPlan || meta.execution_plan)
                : []
        };
    }

    assistantModeLabel(mode) {
        const labels = {
            aiops: 'AI Ops',
            multi_agent: 'Multi-Agent',
            legacy: 'Legacy',
            cache: 'Cache',
            degraded: 'Degraded'
        };
        return labels[mode] || '';
    }

    persistAssistantHistory(content, meta = {}) {
        const normalizedMeta = this.normalizeAssistantMeta(meta);
        const historyItem = {
            type: 'assistant',
            content: content,
            timestamp: new Date().toISOString()
        };
        if (
            normalizedMeta.mode ||
            normalizedMeta.traceId ||
            normalizedMeta.details.length > 0 ||
            normalizedMeta.approvalRequired ||
            normalizedMeta.approvalRequestId
        ) {
            historyItem.meta = normalizedMeta;
        }
        this.currentChatHistory.push(historyItem);
    }

    renderAssistantMeta(messageElement, messageContentWrapper, meta = {}) {
        const normalizedMeta = this.normalizeAssistantMeta(meta);
        const modeLabel = this.assistantModeLabel(normalizedMeta.mode);
        const traceId = normalizedMeta.traceId;

        let metaContainer = messageElement.querySelector('.aiops-meta');
        let traceContainer = messageElement.querySelector('.aiops-trace');
        const messageContent = messageContentWrapper.querySelector('.message-content');

        if (!modeLabel && !traceId) {
            if (metaContainer) {
                metaContainer.remove();
            }
            if (traceContainer) {
                traceContainer.remove();
            }
            return;
        }

        if (!metaContainer) {
            metaContainer = document.createElement('div');
            metaContainer.className = 'aiops-meta';
            messageContentWrapper.insertBefore(metaContainer, messageContentWrapper.firstChild);
        }

        metaContainer.innerHTML = '';

        if (modeLabel) {
            const modePill = document.createElement('div');
            modePill.className = `assistant-mode-pill assistant-mode-${this.escapeHtml(normalizedMeta.mode || 'default')}`;
            modePill.textContent = modeLabel;
            metaContainer.appendChild(modePill);
        }

        if (!traceId) {
            if (traceContainer) {
                traceContainer.remove();
            }
            return;
        }

        if (!traceContainer) {
            traceContainer = document.createElement('div');
            traceContainer.className = 'aiops-trace';
            if (messageContent) {
                messageContentWrapper.insertBefore(traceContainer, messageContent);
            } else {
                messageContentWrapper.appendChild(traceContainer);
            }
        }

        const tracePill = document.createElement('div');
        tracePill.className = 'aiops-trace-pill';
        tracePill.innerHTML = `
            <span class="aiops-meta-label">Trace ID</span>
            <code>${this.escapeHtml(traceId)}</code>
        `;

        const traceButton = document.createElement('button');
        traceButton.type = 'button';
        traceButton.className = 'aiops-trace-btn';
        traceButton.textContent = '查看 Trace';

        let traceToggle = traceContainer.querySelector('.details-toggle');
        let traceContent = traceContainer.querySelector('.details-content');
        if (!traceToggle) {
            traceToggle = document.createElement('div');
            traceToggle.className = 'details-toggle trace-toggle';
            traceToggle.innerHTML = `
                <svg class="toggle-icon" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                    <path d="M9 18L15 12L9 6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
                <span>查看 Trace</span>
            `;

            traceContent = document.createElement('div');
            traceContent.className = 'details-content trace-content';

            traceToggle.addEventListener('click', async () => {
                const expanded = traceContent.classList.contains('expanded');
                if (expanded) {
                    traceContent.classList.remove('expanded');
                    traceToggle.classList.remove('expanded');
                    traceButton.classList.remove('expanded');
                    traceButton.textContent = '查看 Trace';
                    return;
                }

                traceContent.classList.add('expanded');
                traceToggle.classList.add('expanded');
                traceButton.classList.add('expanded');
                traceButton.textContent = '收起 Trace';

                if (traceContent.dataset.loaded === 'true') {
                    return;
                }

                traceContent.innerHTML = '<div class="detail-item">正在加载 Trace...</div>';
                try {
                    const traceData = await this.fetchAIOpsTrace(traceId);
                    const events = Array.isArray(traceData.events) ? traceData.events : [];
                    traceContent.innerHTML = '';

                    if (events.length === 0) {
                        traceContent.innerHTML = '<div class="detail-item">当前没有可显示的 Trace 事件。</div>';
                    } else {
                        events.forEach((event, index) => {
                            const eventItem = document.createElement('div');
                            eventItem.className = 'detail-item trace-item';
                            const agent = this.escapeHtml(event.agent || 'unknown');
                            const type = this.escapeHtml(event.type || 'task_info');
                            const message = this.escapeHtml(event.message || '无消息');
                            const createdAt = this.formatAIOpsTraceTime(event.created_at);
                            eventItem.innerHTML = `
                                <div class="trace-item-head">
                                    <span class="trace-agent">${agent}</span>
                                    <span class="trace-type">${type}</span>
                                    <span class="trace-time">${createdAt}</span>
                                </div>
                                <div class="trace-item-body"><strong>事件 ${index + 1}:</strong> ${message}</div>
                            `;
                            traceContent.appendChild(eventItem);
                        });
                    }
                    traceContent.dataset.loaded = 'true';
                } catch (error) {
                    traceContent.innerHTML = `<div class="detail-item">Trace 加载失败：${this.escapeHtml(error.message || '未知错误')}</div>`;
                    this.showNotification('Trace 加载失败: ' + (error.message || '未知错误'), 'error');
                }
            });

            traceContainer.appendChild(traceToggle);
            traceContainer.appendChild(traceContent);
        } else {
            if (traceContent) {
                traceContent.dataset.loaded = 'false';
            }
        }

        traceButton.addEventListener('click', () => traceToggle.click());
        metaContainer.appendChild(tracePill);
        metaContainer.appendChild(traceButton);
    }

    renderAssistantDetails(messageElement, messageContentWrapper, details = [], title = '查看执行步骤') {
        let detailsContainer = messageElement.querySelector('.aiops-details');
        const messageContent = messageContentWrapper.querySelector('.message-content');

        if (!details || details.length === 0) {
            if (detailsContainer) {
                detailsContainer.remove();
            }
            return;
        }

        if (!detailsContainer) {
            detailsContainer = document.createElement('div');
            detailsContainer.className = 'aiops-details';
            messageContentWrapper.insertBefore(detailsContainer, messageContent);
        } else {
            detailsContainer.innerHTML = '';
        }

        const detailsToggle = document.createElement('div');
        detailsToggle.className = 'details-toggle';
        detailsToggle.innerHTML = `
            <svg class="toggle-icon" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M9 18L15 12L9 6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
            <span>${this.escapeHtml(title)} (${details.length}条)</span>
        `;

        const detailsContent = document.createElement('div');
        detailsContent.className = 'details-content';

        details.forEach((detail, index) => {
            const detailItem = document.createElement('div');
            detailItem.className = 'detail-item';
            detailItem.innerHTML = `<strong>步骤 ${index + 1}:</strong> ${this.escapeHtml(detail)}`;
            detailsContent.appendChild(detailItem);
        });

        detailsToggle.addEventListener('click', () => {
            detailsContent.classList.toggle('expanded');
            detailsToggle.classList.toggle('expanded');
        });

        detailsContainer.appendChild(detailsToggle);
        detailsContainer.appendChild(detailsContent);
    }

    renderApprovalPanel(messageElement, messageContentWrapper, meta = {}) {
        const normalizedMeta = this.normalizeAssistantMeta(meta);
        let container = messageElement.querySelector('.approval-panel');
        const messageContent = messageContentWrapper.querySelector('.message-content');

        if (
            normalizedMeta.mode !== 'aiops' ||
            (
                !normalizedMeta.approvalRequired &&
                !normalizedMeta.approvalStatus &&
                normalizedMeta.executionPlan.length === 0
            )
        ) {
            if (container) {
                container.remove();
            }
            return;
        }

        if (!container) {
            container = document.createElement('div');
            container.className = 'approval-panel';
            messageContentWrapper.insertBefore(container, messageContent);
        }

        container.innerHTML = '';

        const header = document.createElement('div');
        header.className = 'approval-panel-header';

        const title = document.createElement('div');
        title.className = 'approval-panel-title';
        title.textContent = normalizedMeta.approvalRequired ? '待审批执行计划' : '执行计划';
        header.appendChild(title);

        if (normalizedMeta.approvalStatus) {
            const status = document.createElement('span');
            status.className = `approval-status-pill approval-status-${this.escapeHtml(normalizedMeta.approvalStatus)}`;
            status.textContent = normalizedMeta.approvalStatus;
            header.appendChild(status);
        }

        container.appendChild(header);

        if (normalizedMeta.executionPlan.length > 0) {
            const planList = document.createElement('ol');
            planList.className = 'approval-plan-list';
            normalizedMeta.executionPlan.forEach((step) => {
                const item = document.createElement('li');
                item.textContent = step;
                planList.appendChild(item);
            });
            container.appendChild(planList);
        }

        if (normalizedMeta.degradationReason) {
            const note = document.createElement('div');
            note.className = 'approval-panel-note';
            note.textContent = normalizedMeta.degradationReason;
            container.appendChild(note);
        }

        const canApprove = normalizedMeta.approvalRequired &&
            normalizedMeta.approvalRequestId &&
            (!normalizedMeta.approvalStatus || normalizedMeta.approvalStatus === 'pending');
        if (!canApprove) {
            return;
        }

        const actionBar = document.createElement('div');
        actionBar.className = 'approval-action-bar';

        const approveButton = document.createElement('button');
        approveButton.type = 'button';
        approveButton.className = 'approval-action-btn';
        approveButton.textContent = '批准并执行';
        approveButton.addEventListener('click', async () => {
            approveButton.disabled = true;
            approveButton.textContent = '执行中...';
            try {
                await this.approveApprovalRequest(normalizedMeta.approvalRequestId, messageElement);
                this.showNotification('审批已通过，原请求开始执行', 'success');
            } catch (error) {
                approveButton.disabled = false;
                approveButton.textContent = '批准并执行';
                this.showNotification('审批执行失败: ' + (error.message || '未知错误'), 'error');
            }
        });

        actionBar.appendChild(approveButton);
        container.appendChild(actionBar);
    }

    addAssistantMessageWithMeta(response, meta = {}, saveToHistory = true) {
        const normalizedMeta = this.normalizeAssistantMeta(meta);
        const messageDiv = document.createElement('div');
        messageDiv.className = `message assistant${normalizedMeta.mode === 'aiops' ? ' aiops-message' : ''}`;

        const messageAvatar = document.createElement('div');
        messageAvatar.className = 'message-avatar';
        messageAvatar.innerHTML = `
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M12 2L15.09 8.26L22 9.27L17 14.14L18.18 21.02L12 17.77L5.82 21.02L7 14.14L2 9.27L8.91 8.26L12 2Z" fill="white"/>
            </svg>
        `;
        messageDiv.appendChild(messageAvatar);

        const messageContentWrapper = document.createElement('div');
        messageContentWrapper.className = 'message-content-wrapper';

        const messageContent = document.createElement('div');
        messageContent.className = 'message-content';
        messageContent.innerHTML = this.renderMarkdown(response);
        this.highlightCodeBlocks(messageContent);

        messageContentWrapper.appendChild(messageContent);
        this.renderAssistantMeta(messageDiv, messageContentWrapper, normalizedMeta);
        this.renderAssistantDetails(
            messageDiv,
            messageContentWrapper,
            normalizedMeta.details,
            normalizedMeta.mode === 'aiops' ? '查看详细步骤' : '查看执行步骤'
        );
        this.renderApprovalPanel(messageDiv, messageContentWrapper, normalizedMeta);
        this.appendMessageActions(messageContentWrapper, 'assistant', response);
        messageDiv.appendChild(messageContentWrapper);

        if (saveToHistory) {
            this.persistAssistantHistory(response, normalizedMeta);
        }

        if (this.chatMessages) {
            this.chatMessages.appendChild(messageDiv);
            this.scrollToBottom();
        }

        return messageDiv;
    }

    // 添加带加载动画的消息
    addLoadingMessage(content) {
        const messageDiv = document.createElement('div');
        messageDiv.className = 'message assistant';

        // 添加头像图标
        const messageAvatar = document.createElement('div');
        messageAvatar.className = 'message-avatar';
        messageAvatar.innerHTML = `
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M12 2L15.09 8.26L22 9.27L17 14.14L18.18 21.02L12 17.77L5.82 21.02L7 14.14L2 9.27L8.91 8.26L12 2Z" fill="white"/>
            </svg>
        `;
        messageDiv.appendChild(messageAvatar);

        // 创建消息内容包装器
        const messageContentWrapper = document.createElement('div');
        messageContentWrapper.className = 'message-content-wrapper';

        const messageContent = document.createElement('div');
        messageContent.className = 'message-content loading-message-content';
        
        // 创建文本和动画容器
        const textSpan = document.createElement('span');
        textSpan.textContent = content;
        
        // 创建旋转动画图标
        const loadingIcon = document.createElement('span');
        loadingIcon.className = 'loading-spinner-icon';
        loadingIcon.innerHTML = `
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8z" fill="currentColor" opacity="0.2"/>
                <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10c1.54 0 3-.36 4.28-1l-1.5-2.6C13.64 19.62 12.84 20 12 20c-4.41 0-8-3.59-8-8s3.59-8 8-8c.84 0 1.64.38 2.18 1l1.5-2.6C13 2.36 12.54 2 12 2z" fill="currentColor"/>
            </svg>
        `;
        
        messageContent.appendChild(textSpan);
        messageContent.appendChild(loadingIcon);
        messageContentWrapper.appendChild(messageContent);
        messageDiv.appendChild(messageContentWrapper);

        if (this.chatMessages) {
            this.chatMessages.appendChild(messageDiv);
            
            // 如果是第一条消息，移除居中样式
            const isFirstMessage = this.chatMessages.querySelectorAll('.message').length === 1;
            if (isFirstMessage && this.chatContainer) {
                this.chatContainer.classList.remove('centered');
                this.chatContainer.style.transition = 'all 0.5s ease';
            }
            
            this.scrollToBottom();
        }

        return messageDiv;
    }
    
    // 检查并设置居中样式
    applyStoredTheme() {
        const stored = this.readStorage('opscaptain-theme-v2');
        if (stored === 'dark') {
            document.documentElement.setAttribute('data-theme', 'dark');
            this.updateThemeIcons('dark');
            return;
        }
        document.documentElement.setAttribute('data-theme', 'light');
        this.updateThemeIcons('light');
    }

    toggleTheme() {
        const isLight = document.documentElement.getAttribute('data-theme') === 'light';
        if (isLight) {
            document.documentElement.setAttribute('data-theme', 'dark');
            this.writeStorage('opscaptain-theme-v2', 'dark');
            this.updateThemeIcons('dark');
        } else {
            document.documentElement.setAttribute('data-theme', 'light');
            this.writeStorage('opscaptain-theme-v2', 'light');
            this.updateThemeIcons('light');
        }
    }

    readStorage(key) {
        try {
            return localStorage.getItem(key);
        } catch (error) {
            console.warn('读取本地存储失败:', key, error);
            return null;
        }
    }

    writeStorage(key, value) {
        try {
            localStorage.setItem(key, value);
            return true;
        } catch (error) {
            console.warn('写入本地存储失败:', key, error);
            return false;
        }
    }

    updateThemeIcons(theme) {
        if (!this.themeIconMoon || !this.themeIconSun) return;
        const hljsLink = document.getElementById('hljsTheme');
        if (theme === 'light') {
            this.themeIconMoon.style.display = 'none';
            this.themeIconSun.style.display = 'block';
            if (hljsLink) hljsLink.href = 'https://cdn.jsdelivr.net/npm/highlight.js@11.9.0/styles/github.min.css';
        } else {
            this.themeIconMoon.style.display = 'block';
            this.themeIconSun.style.display = 'none';
            if (hljsLink) hljsLink.href = 'https://cdn.jsdelivr.net/npm/highlight.js@11.9.0/styles/github-dark.min.css';
        }
    }

    addThinkingBubble() {
        const messageDiv = document.createElement('div');
        messageDiv.className = 'message assistant';
        messageDiv.id = 'thinking-bubble';

        const messageAvatar = document.createElement('div');
        messageAvatar.className = 'message-avatar';
        messageAvatar.innerHTML = `
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M12 2L15.09 8.26L22 9.27L17 14.14L18.18 21.02L12 17.77L5.82 21.02L7 14.14L2 9.27L8.91 8.26L12 2Z" fill="white"/>
            </svg>
        `;
        messageDiv.appendChild(messageAvatar);

        const messageContentWrapper = document.createElement('div');
        messageContentWrapper.className = 'message-content-wrapper';

        const bubble = document.createElement('div');
        bubble.className = 'thinking-bubble';
        bubble.innerHTML = `
            <div class="thinking-dots"><span></span><span></span><span></span></div>
        `;

        messageContentWrapper.appendChild(bubble);
        messageDiv.appendChild(messageContentWrapper);

        if (this.chatMessages) {
            this.chatMessages.appendChild(messageDiv);

            const isFirstMessage = this.chatMessages.querySelectorAll('.message').length === 1;
            if (isFirstMessage && this.chatContainer) {
                this.chatContainer.classList.remove('centered');
                this.chatContainer.style.transition = 'all 0.5s ease';
            }

            this.scrollToBottom();
        }

        return messageDiv;
    }

    removeThinkingBubble() {
        const bubble = document.getElementById('thinking-bubble');
        if (bubble) {
            bubble.remove();
        }
    }

    checkAndSetCentered() {
        if (this.chatMessages && this.chatContainer) {
            const hasMessages = this.chatMessages.querySelectorAll('.message').length > 0;
            if (!hasMessages) {
                this.chatContainer.classList.add('centered');
                document.body.classList.add('landing-mode');
            } else {
                this.chatContainer.classList.remove('centered');
                document.body.classList.remove('landing-mode');
            }
        }
    }

    // 滚动到底部
    scrollToBottom() {
        if (this.chatMessages) {
            this.chatMessages.scrollTop = this.chatMessages.scrollHeight;
        }
    }

    // 显示通知
    showNotification(message, type = 'info') {
        // 创建通知元素
        const notification = document.createElement('div');
        notification.className = `notification ${type}`;
        notification.textContent = message;
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            padding: 15px 20px;
            border-radius: 8px;
            color: white;
            font-weight: 500;
            z-index: 10000;
            animation: slideIn 0.3s ease;
            max-width: 300px;
        `;

        // 根据类型设置颜色（Google Material Design配色）
        const colors = {
            info: '#1a73e8',
            success: '#34a853',
            warning: '#fbbc04',
            error: '#ea4335'
        };
        notification.style.backgroundColor = colors[type] || colors.info;

        // 添加到页面
        document.body.appendChild(notification);

        // 3秒后自动移除
        setTimeout(() => {
            notification.style.animation = 'slideOut 0.3s ease';
            setTimeout(() => {
                if (notification.parentNode) {
                    notification.parentNode.removeChild(notification);
                }
            }, 300);
        }, 3000);
    }

    // 处理文件选择
    handleFileSelect(event) {
        const file = event.target.files[0];
        if (file) {
            // 验证文件格式
            if (!this.validateFileType(file)) {
                this.showNotification(`仅支持 ${this.allowedUploadExtensions.join(', ')} 文件`, 'error');
                this.fileInput.value = '';
                return;
            }
            this.uploadFile(file);
        }
    }

    // 验证文件类型
    validateFileType(file) {
        const fileName = file.name.toLowerCase();
        return this.allowedUploadExtensions.some(ext => fileName.endsWith(ext));
    }

    // 上传文件到知识库
    async uploadFile(file) {
        // 再次验证文件类型（双重保险）
        if (!this.validateFileType(file)) {
            this.showNotification(`仅支持 ${this.allowedUploadExtensions.join(', ')} 文件`, 'error');
            return;
        }

        // 验证文件大小（与后端保持一致）
        const maxSize = this.maxUploadSizeMB * 1024 * 1024;
        if (file.size > maxSize) {
            this.showNotification(`文件大小不能超过 ${this.maxUploadSizeMB}MB`, 'error');
            return;
        }

        // 锁定前端并显示上传遮罩层
        this.isStreaming = true;
        this.createAbortController();
        this.updateUI();
        this.showUploadOverlay(true, file.name);

        try {
            // 创建 FormData
            const formData = new FormData();
            formData.append('file', file);

            // 发送上传请求
            const response = await this.apiFetch('/upload', {
                method: 'POST',
                signal: this.abortController ? this.abortController.signal : undefined,
                body: formData
            });

            if (!response.ok) {
                throw new Error(`HTTP错误: ${response.status}`);
            }

            const data = await response.json();

            if (data.message === 'OK' && data.data) {
                // 在聊天界面显示上传成功消息
                const successMessage = `${file.name} 上传到知识库成功`;
                this.addMessage('assistant', successMessage, false, true);
            } else {
                throw new Error(data.message || '上传失败');
            }
        } catch (error) {
            if (!this.isAbortError(error)) {
                console.error('文件上传失败:', error);
                this.showNotification('文件上传失败: ' + error.message, 'error');
            }
        } finally {
            // 清空文件输入
            if (this.fileInput) {
                this.fileInput.value = '';
            }
            // 解锁前端
            this.isStreaming = false;
            this.clearAbortController();
            this.showUploadOverlay(false);
            this.updateUI();
        }
    }

    // 格式化文件大小
    formatFileSize(bytes) {
        if (bytes === 0) return '0 Bytes';
        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
    }

    // 发送智能运维请求
    async sendAIOpsRequest(loadingMessageElement) {
        try {
            const response = await this.apiFetch('/ai_ops', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                signal: this.abortController ? this.abortController.signal : undefined
            });

            if (!response.ok) {
                throw new Error(`HTTP错误: ${response.status}`);
            }

            const data = await response.json();
            
            if (data.message === 'OK' && data.data) {
                // 解析Result中的response字段
                let responseText = '';
                try {
                    const resultObj = JSON.parse(data.data.result);
                    responseText = resultObj.response || data.data.result;
                } catch (e) {
                    // 如果解析失败，直接使用result
                    responseText = data.data.result;
                }
                
                // 更新消息内容
                this.updateAIOpsMessage(
                    loadingMessageElement,
                    responseText,
                    data.data || {}
                );
            } else {
                throw new Error(data.message || '未知错误');
            }
        } catch (error) {
            if (this.isAbortError(error)) {
                return;
            }
            throw error;
        }
    }

    async fetchAIOpsTrace(traceId) {
        const response = await this.apiFetch(`/ai_ops_trace?trace_id=${encodeURIComponent(traceId)}`);
        if (!response.ok) {
            throw new Error(`HTTP错误: ${response.status}`);
        }

        const data = await response.json();
        if (data.message === 'OK' && data.data) {
            return data.data;
        }
        throw new Error(data.message || 'Trace查询失败');
    }

    async approveApprovalRequest(requestId, messageElement) {
        const response = await this.apiFetch('/approval_requests/approve', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                request_id: requestId
            })
        });

        if (!response.ok) {
            throw new Error(`HTTP错误: ${response.status}`);
        }

        const data = await response.json();
        if (!(data.message === 'OK' && data.data)) {
            throw new Error(data.message || '审批执行失败');
        }

        const payload = data.data;
        this.updateAIOpsMessage(messageElement, payload.result || '', payload);
        return payload;
    }

    formatAIOpsTraceTime(timestamp) {
        if (!timestamp) {
            return '--:--:--';
        }
        try {
            return new Date(timestamp).toLocaleTimeString('zh-CN', {
                hour12: false,
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit'
            });
        } catch (error) {
            return String(timestamp);
        }
    }

    // 更新智能运维消息（带折叠详情）
    updateAIOpsMessage(messageElement, response, meta = {}) {
        if (!messageElement) {
            // 如果没有传入消息元素，则创建新消息
            return this.addAIOpsMessage(response, meta);
        }

        // 添加aiops-message类
        messageElement.classList.add('aiops-message');

        // 获取消息内容包装器
        const messageContentWrapper = messageElement.querySelector('.message-content-wrapper');
        if (!messageContentWrapper) {
            return;
        }

        const normalizedMeta = this.normalizeAssistantMeta({
            mode: 'aiops',
            ...meta
        });
        this.renderAssistantMeta(messageElement, messageContentWrapper, normalizedMeta);

        // 清空现有内容（保留消息内容容器）
        const messageContent = messageContentWrapper.querySelector('.message-content');
        if (!messageContent) {
            return;
        }

        // 移除加载动画相关的类和内容
        messageContent.classList.remove('loading-message-content');
        messageContent.textContent = '';
        
        // 移除加载图标（如果存在）
        const loadingIcon = messageContent.querySelector('.loading-spinner-icon');
        if (loadingIcon) {
            loadingIcon.remove();
        }

        // 详情部分（可折叠）- 先显示
        this.renderAssistantDetails(messageElement, messageContentWrapper, normalizedMeta.details || [], '查看详细步骤');
        this.renderApprovalPanel(messageElement, messageContentWrapper, normalizedMeta);

        // 更新主要响应内容（使用Markdown渲染）
        messageContent.innerHTML = this.renderMarkdown(response);
        // 高亮代码块
        this.highlightCodeBlocks(messageContent);
        this.appendMessageActions(messageContentWrapper, 'assistant', response);
        
        // 保存到历史记录
        this.persistAssistantHistory(response, normalizedMeta);
        
        this.scrollToBottom();
        return messageElement;
    }

    // 添加智能运维消息（带折叠详情）- 保留用于兼容性
    addAIOpsMessage(response, meta = {}) {
        return this.addAssistantMessageWithMeta(response, {
            mode: 'aiops',
            ...meta
        });
    }

    // HTML转义
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // 触发智能运维（点击智能运维按钮时直接调用）
    async triggerAIOps() {
        if (this.isStreaming) {
            this.showNotification('请等待当前操作完成', 'warning');
            return;
        }

        // 新建对话
        this.newChat();
        
        // 添加"分析中..."的消息（带旋转动画）
        const loadingMessage = this.addLoadingMessage('分析中...');
        this.currentAIOpsMessage = loadingMessage; // 保存消息引用用于后续更新
        
        // 设置发送状态
        this.isStreaming = true;
        this.createAbortController();
        this.updateUI();

        try {
            await this.sendAIOpsRequest(loadingMessage);
        } catch (error) {
            if (!this.isAbortError(error)) {
                console.error('智能运维分析失败:', error);
                // 更新消息为错误信息
                if (loadingMessage) {
                    const messageContent = loadingMessage.querySelector('.message-content');
                    if (messageContent) {
                        messageContent.textContent = '抱歉，智能运维分析时出现错误：' + error.message;
                    }
                }
            }
        } finally {
            this.isStreaming = false;
            this.currentAIOpsMessage = null;
            this.clearAbortController();
            this.updateUI();
        }
    }

    // 显示/隐藏加载遮罩层
    showLoadingOverlay(show) {
        if (this.loadingOverlay) {
            if (show) {
                this.loadingOverlay.style.display = 'flex';
                // 更新文字为智能运维
                const loadingText = this.loadingOverlay.querySelector('.loading-text');
                const loadingSubtext = this.loadingOverlay.querySelector('.loading-subtext');
                if (loadingText) loadingText.textContent = '智能运维分析中，请稍候...';
                if (loadingSubtext) loadingSubtext.textContent = '后端正在处理，请耐心等待';
                // 防止页面滚动
                document.body.style.overflow = 'hidden';
            } else {
                this.loadingOverlay.style.display = 'none';
                // 恢复页面滚动
                document.body.style.overflow = '';
            }
        }
    }

    // 显示/隐藏上传遮罩层
    showUploadOverlay(show, fileName = '') {
        if (this.loadingOverlay) {
            if (show) {
                this.loadingOverlay.style.display = 'flex';
                // 更新文字为上传中
                const loadingText = this.loadingOverlay.querySelector('.loading-text');
                const loadingSubtext = this.loadingOverlay.querySelector('.loading-subtext');
                if (loadingText) loadingText.textContent = '正在上传文件...';
                if (loadingSubtext) loadingSubtext.textContent = fileName ? `上传: ${fileName}` : '请稍候';
                // 防止页面滚动
                document.body.style.overflow = 'hidden';
            } else {
                this.loadingOverlay.style.display = 'none';
                // 恢复页面滚动
                document.body.style.overflow = '';
            }
        }
    }
}

// 添加CSS动画
const style = document.createElement('style');
style.textContent = `
    @keyframes slideIn {
        from {
            transform: translateX(100%);
            opacity: 0;
        }
        to {
            transform: translateX(0);
            opacity: 1;
        }
    }
    
    @keyframes slideOut {
        from {
            transform: translateX(0);
            opacity: 1;
        }
        to {
            transform: translateX(100%);
            opacity: 0;
        }
    }
`;
document.head.appendChild(style);

// 初始化应用
function bootstrapSuperBizAgentApp() {
    if (window.__superBizAgentAppBooted) {
        return;
    }
    try {
        const app = new SuperBizAgentApp();
        window.__superBizAgentApp = app;
        window.__superBizAgentAppBooted = true;
    } catch (error) {
        window.__superBizAgentAppBooted = false;
        window.__superBizAgentApp = null;
        console.error('SuperBizAgentApp 初始化失败:', error);
    }
}

document.addEventListener('DOMContentLoaded', bootstrapSuperBizAgentApp);

if (document.readyState !== 'loading') {
    bootstrapSuperBizAgentApp();
}
