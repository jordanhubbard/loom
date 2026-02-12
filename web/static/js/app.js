// Configuration
const API_BASE = '/api/v1';
const REFRESH_INTERVAL = 5000; // 5 seconds

const AUTH_TOKEN_KEY = 'loom.authToken';
let authToken = localStorage.getItem(AUTH_TOKEN_KEY) || '';
let authCheckInFlight = null;
let loginInFlight = null;

// Feature flag: Set to false to disable authentication in the UI
// Should match server-side config.yaml security.enable_auth setting
const AUTH_ENABLED = false;

// State
let state = {
    beads: [],
    ceoBeads: null,
    agents: [],
    projects: [],
    personas: [],
    decisions: [],
    providers: [],
    systemStatus: null,
    users: [],
    apiKeys: []
};

let uiState = {
    view: {
        active: 'project-viewer'
    },
    bead: {
        search: '',
        sort: 'priority',
        priority: 'all',
        type: 'all',
        assigned: '',
        tag: ''
    },
    agent: {
        search: ''
    },
    project: {
        selectedId: ''
    }
};

let busy = new Set();

// Exposed on window so hotreload.js can check if a modal is open
window.modalState = {
    activeId: null,
    lastFocused: null
};
const modalState = window.modalState;

// Helper to get authentication headers for API calls
function getAuthHeaders() {
    const headers = { 'Content-Type': 'application/json' };
    if (AUTH_ENABLED && authToken) {
        headers['Authorization'] = `Bearer ${authToken}`;
    }
    return headers;
}

let eventStreamConnected = false;
let reloadTimers = {};

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    console.log('[Loom] DOMContentLoaded - Initializing...');
    console.log('[Loom] AUTH_ENABLED =', AUTH_ENABLED);
    try {
        initUI();
        initViewTabs();
        // Initialize diagrams UI after Cytoscape loads
        if (typeof cytoscape !== 'undefined' && typeof initDiagramsUI === 'function') {
            initDiagramsUI();
        } else {
            console.warn('[Loom] Cytoscape.js or diagrams.js not loaded');
        }
        loadAll();
        startEventStream();
        startAutoRefresh();
        console.log('[Loom] Initialization complete');
    } catch (error) {
        console.error('[Loom] Initialization failed:', error);
    }
});

function initUI() {
    const beadSearch = document.getElementById('bead-search');
    const beadSort = document.getElementById('bead-sort');
    const beadPriority = document.getElementById('bead-priority');
    const beadType = document.getElementById('bead-type');
    const beadAssigned = document.getElementById('bead-assigned');
    const beadTag = document.getElementById('bead-tag');
    const beadClear = document.getElementById('bead-clear-filters');

    // User management controls
    const createUserBtn = document.getElementById('create-user-btn');
    const cancelUserBtn = document.getElementById('cancel-user-btn');
    const userForm = document.getElementById('user-form');
    const refreshUsersBtn = document.getElementById('refresh-users-btn');
    
    createUserBtn?.addEventListener('click', () => {
        const form = document.getElementById('create-user-form');
        if (form) form.style.display = 'block';
    });
    
    cancelUserBtn?.addEventListener('click', () => {
        const form = document.getElementById('create-user-form');
        if (form) form.style.display = 'none';
        document.getElementById('user-form')?.reset();
    });
    
    userForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        await handleCreateUser();
    });
    
    refreshUsersBtn?.addEventListener('click', () => {
        loadUsers();
        loadAPIKeys();
        render();
    });

    // API key controls
    const createAPIKeyBtn = document.getElementById('create-apikey-btn');
    const cancelAPIKeyBtn = document.getElementById('cancel-apikey-btn');
    const apiKeyForm = document.getElementById('apikey-form');
    const closeAPIKeyDisplayBtn = document.getElementById('close-apikey-display-btn');
    const copyAPIKeyBtn = document.getElementById('copy-apikey-btn');
    
    createAPIKeyBtn?.addEventListener('click', () => {
        const form = document.getElementById('create-apikey-form');
        if (form) form.style.display = 'block';
    });
    
    cancelAPIKeyBtn?.addEventListener('click', () => {
        const form = document.getElementById('create-apikey-form');
        if (form) form.style.display = 'none';
        document.getElementById('apikey-form')?.reset();
    });
    
    apiKeyForm?.addEventListener('submit', async (e) => {
        e.preventDefault();
        await handleCreateAPIKey();
    });
    
    closeAPIKeyDisplayBtn?.addEventListener('click', () => {
        const display = document.getElementById('apikey-display');
        if (display) display.style.display = 'none';
    });
    
    copyAPIKeyBtn?.addEventListener('click', () => {
        const keyValue = document.getElementById('apikey-value')?.textContent;
        if (keyValue) {
            navigator.clipboard.writeText(keyValue).then(() => {
                showToast('API key copied to clipboard', 'success');
            });
        }
    });

    beadSearch?.addEventListener('input', (e) => {
        uiState.bead.search = e.target.value || '';
        render();
    });
    beadSort?.addEventListener('change', (e) => {
        uiState.bead.sort = e.target.value;
        render();
    });
    beadPriority?.addEventListener('change', (e) => {
        uiState.bead.priority = e.target.value;
        render();
    });
    beadType?.addEventListener('change', (e) => {
        uiState.bead.type = e.target.value;
        render();
    });
    beadAssigned?.addEventListener('input', (e) => {
        uiState.bead.assigned = e.target.value || '';
        render();
    });
    beadTag?.addEventListener('input', (e) => {
        uiState.bead.tag = e.target.value || '';
        render();
    });

    beadClear?.addEventListener('click', () => {
        uiState.bead = {
            search: '',
            sort: 'priority',
            priority: 'all',
            type: 'all',
            assigned: '',
            tag: ''
        };

        if (beadSearch) beadSearch.value = '';
        if (beadSort) beadSort.value = 'priority';
        if (beadPriority) beadPriority.value = 'all';
        if (beadType) beadType.value = 'all';
        if (beadAssigned) beadAssigned.value = '';
        if (beadTag) beadTag.value = '';

        render();
    });

    const agentSearch = document.getElementById('agent-search');
    agentSearch?.addEventListener('input', (e) => {
        uiState.agent.search = e.target.value || '';
        render();
    });

    setupNavActiveState();

    initKanbanDnD();


    const projectSelect = document.getElementById('project-view-select');
    projectSelect?.addEventListener('change', (e) => {
        uiState.project.selectedId = e.target.value || '';
        render();
    });

    // Legacy REPL (backward compat)
    const replSend = document.getElementById('repl-send');
    replSend?.addEventListener('click', () => {
        sendReplQuery();
    });

    // Unified CEO REPL
    const ceoReplSend = document.getElementById('ceo-repl-send');
    ceoReplSend?.addEventListener('click', () => {
        sendCeoReplQuery();
    });

    // Streaming test controls
    const streamTestSend = document.getElementById('stream-test-send');
    streamTestSend?.addEventListener('click', () => {
        sendStreamingTest();
    });

    const streamTestClear = document.getElementById('stream-test-clear');
    streamTestClear?.addEventListener('click', () => {
        const responseEl = document.getElementById('stream-test-response');
        const statsEl = document.getElementById('stream-test-stats');
        if (responseEl) responseEl.textContent = '';
        if (statsEl) statsEl.textContent = '';
    });
}

function setupNavActiveState() {
    const links = Array.from(document.querySelectorAll('.nav-list a'));
    const sectionIds = links
        .map((a) => (a.getAttribute('href') || '').replace('#', ''))
        .filter(Boolean);

    function setActive(id) {
        for (const a of links) {
            const targetId = (a.getAttribute('href') || '').replace('#', '');
            if (targetId === id) {
                a.setAttribute('aria-current', 'page');
            } else {
                a.removeAttribute('aria-current');
            }
        }
    }

    window.addEventListener('hashchange', () => {
        const id = (location.hash || '').replace('#', '');
        if (id) setActive(id);
    });

    const sections = sectionIds
        .map((id) => document.getElementById(id))
        .filter(Boolean);

    if (sections.length === 0) return;

    const observer = new IntersectionObserver(
        (entries) => {
            const visible = entries
                .filter((e) => e.isIntersecting)
                .sort((a, b) => (b.intersectionRatio || 0) - (a.intersectionRatio || 0));
            if (visible.length > 0) setActive(visible[0].target.id);
        },
        { rootMargin: '-40% 0px -55% 0px', threshold: [0, 0.2, 0.4, 0.6] }
    );

    for (const s of sections) observer.observe(s);
}

function initViewTabs() {
    const tabs = Array.from(document.querySelectorAll('.view-tab'));
    const panels = Array.from(document.querySelectorAll('.view-panel'));

    function activate(id) {
        uiState.view.active = id;
        for (const tab of tabs) {
            const target = tab.getAttribute('data-target');
            const isActive = target === id;
            tab.classList.toggle('active', isActive);
            tab.setAttribute('aria-selected', isActive ? 'true' : 'false');
            tab.tabIndex = isActive ? 0 : -1;
        }
        for (const panel of panels) {
            const isActive = panel.id === id;
            panel.classList.toggle('active', isActive);
            if (isActive) {
                panel.removeAttribute('hidden');
            } else {
                panel.setAttribute('hidden', 'true');
            }
        }
    }

    for (const tab of tabs) {
        tab.addEventListener('click', () => {
            const target = tab.getAttribute('data-target');
            if (target) activate(target);
        });
        tab.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                const target = tab.getAttribute('data-target');
                if (target) activate(target);
            }
        });
    }

    window.addEventListener('hashchange', () => {
        const id = (location.hash || '').replace('#', '');
        if (id && panels.find((p) => p.id === id)) {
            activate(id);
        }
    });

    // Respect hash on load if it matches a panel id
    const hash = (location.hash || '').replace('#', '');
    if (hash && panels.find((p) => p.id === hash)) {
        activate(hash);
    } else {
        activate(uiState.view.active);
    }
}

// Auto-refresh
function startAutoRefresh() {
    // Event bus is preferred; this interval is a fallback.
    setInterval(() => {
        if (!eventStreamConnected && !modalState.activeId) loadAll();
    }, REFRESH_INTERVAL);
}

// Load all data
async function loadAll() {
    console.log('[Loom] loadAll() started');
    try {
        await Promise.all([
            loadBeads().catch(err => { console.error('[Loom] Failed to load beads:', err); state.beads = []; }),
            loadProviders().catch(err => { console.error('[Loom] Failed to load providers:', err); state.providers = []; }),
            loadAgents().catch(err => { console.error('[Loom] Failed to load agents:', err); state.agents = []; }),
            loadProjects().catch(err => { console.error('[Loom] Failed to load projects:', err); state.projects = []; }),
            loadPersonas().catch(err => { console.error('[Loom] Failed to load personas:', err); state.personas = []; }),
            loadDecisions().catch(err => { console.error('[Loom] Failed to load decisions:', err); state.decisions = []; }),
            loadSystemStatus().catch(err => { console.error('[Loom] Failed to load system status:', err); }),
            loadUsers().catch(err => { console.error('[Loom] Failed to load users:', err); state.users = []; }),
            loadAPIKeys().catch(err => { console.error('[Loom] Failed to load API keys:', err); state.apiKeys = []; }),
            loadMotivations().catch(err => { console.error('[Loom] Failed to load motivations:', err); })
        ]);
        await loadCeoBeads().catch(err => { console.error('[Loom] Failed to load CEO beads:', err); state.ceoBeads = []; });
        console.log('[Loom] Data loaded successfully:', {
            beads: state.beads?.length || 0,
            projects: state.projects?.length || 0,
            agents: state.agents?.length || 0,
            providers: state.providers?.length || 0
        });
        render();
        console.log('[Loom] render() completed');
    } catch (error) {
        console.error('[Loom] loadAll() failed:', error);
    }
}

// API calls
async function apiCall(endpoint, options = {}) {
    let autoFiledApiFailure = false;
    try {
        const headers = {
            'Content-Type': 'application/json',
            ...options.headers
        };

        // Only add auth header if authentication is enabled and we have a token
        if (AUTH_ENABLED && !options.skipAuth && authToken) {
            headers.Authorization = `Bearer ${authToken}`;
        }

        const response = await fetch(`${API_BASE}${endpoint}`, {
            ...options,
            headers
        });
        
        if (!response.ok) {
            let message = 'API request failed';
            try {
                const error = await response.json();
                message = error.error || message;
            } catch {
                try {
                    const text = await response.text();
                    if (text) message = text;
                } catch {
                    // ignore
                }
            }
            // Only try to authenticate if auth is enabled
            if (AUTH_ENABLED && response.status === 401 && !options.skipAuth && !options.retryAuth) {
                await ensureAuth(true);
                return apiCall(endpoint, { ...options, retryAuth: true });
            }
            if (!options.skipAutoFile
                && typeof window !== 'undefined'
                && typeof window.fileApiBug === 'function') {
                window.fileApiBug({
                    endpoint,
                    method: options.method || 'GET',
                    status: response.status,
                    message,
                    response: message
                });
                autoFiledApiFailure = true;
            }
            throw new Error(message);
        }
        
        if (response.status === 204) {
            return null;
        }
        
        return await response.json();
    } catch (error) {
        if (!autoFiledApiFailure
            && !options.skipAutoFile
            && typeof window !== 'undefined'
            && typeof window.fileApiBug === 'function') {
            window.fileApiBug({
                endpoint,
                method: options.method || 'GET',
                status: 0,
                message: error.message || 'Network error',
                response: error.message || ''
            });
        }
        console.error('[Loom] API Error:', error);
        if (!options.suppressToast) {
            showToast(error.message || 'Request failed', 'error');
        }
        throw error;
    }
}

async function ensureAuth(forcePrompt = false) {
    // Skip authentication if it's disabled
    if (!AUTH_ENABLED) {
        return;
    }
    
    if (authCheckInFlight) return authCheckInFlight;
    authCheckInFlight = (async () => {
        if (!forcePrompt && authToken) {
            try {
                await apiCall('/auth/me', { skipAuth: false, suppressToast: true, retryAuth: true });
                return;
            } catch (err) {
                // Fall through to prompt.
            }
        }
        await showLoginModal();
    })().finally(() => {
        authCheckInFlight = null;
    });
    return authCheckInFlight;
}

async function showLoginModal() {
    if (loginInFlight) return loginInFlight;
    loginInFlight = (async () => {
        let loggedIn = false;
        while (!loggedIn) {
            const values = await formModal({
                title: 'Sign in',
                submitText: 'Sign in',
                fields: [
                    { id: 'username', label: 'Username', required: true, placeholder: 'admin' },
                    { id: 'password', label: 'Password', required: true, type: 'password', placeholder: 'Password' }
                ]
            });
            if (!values) {
                throw new Error('Login required');
            }
            try {
                const resp = await apiCall('/auth/login', {
                    method: 'POST',
                    body: JSON.stringify({
                        username: (values.username || '').trim(),
                        password: values.password || ''
                    }),
                    skipAuth: true
                });
                if (resp?.token) {
                    authToken = resp.token;
                    localStorage.setItem(AUTH_TOKEN_KEY, authToken);
                    showToast('Signed in', 'success');
                    loggedIn = true;
                } else {
                    throw new Error('Login failed');
                }
            } catch (err) {
                showToast(`Login failed: ${err.message || 'Unknown error'}`, 'error');
            }
        }
    })().finally(() => {
        loginInFlight = null;
    });
    return loginInFlight;
}

function scheduleReload(kind, delayMs = 150) {
    if (reloadTimers[kind]) return;
    reloadTimers[kind] = window.setTimeout(async () => {
        try {
            if (kind === 'beads') await loadBeads();
            if (kind === 'agents') await loadAgents();
            if (kind === 'projects') await loadProjects();
            if (kind === 'providers') await loadProviders();
            if (kind === 'decisions') await loadDecisions();
            if (kind === 'status') await loadSystemStatus();
            if (!modalState.activeId) render();
        } catch (e) {
            // Errors are already surfaced via apiCall toasts.
        } finally {
            window.clearTimeout(reloadTimers[kind]);
            delete reloadTimers[kind];
        }
    }, delayMs);
}

function startEventStream() {
    if (typeof EventSource === 'undefined') return;

    try {
        const es = new EventSource(`${API_BASE}/events/stream`);

        es.addEventListener('connected', () => {
            eventStreamConnected = true;
        });

        const map = {
            'bead.created': ['beads', 'status'],
            'bead.assigned': ['beads', 'agents', 'status'],
            'bead.status_change': ['beads', 'status'],
            'bead.completed': ['beads', 'status'],
            'agent.spawned': ['agents', 'projects', 'status'],
            'agent.status_change': ['agents', 'status'],
            'agent.heartbeat': ['agents', 'status'],
            'agent.completed': ['agents', 'status'],
            'decision.created': ['decisions'],
            'decision.resolved': ['decisions'],
            'provider.registered': ['providers'],
            'provider.deleted': ['providers'],
            'provider.updated': ['providers'],
            'project.created': ['projects'],
            'project.updated': ['projects'],
            'project.deleted': ['projects'],
            'config.updated': ['projects', 'providers', 'agents', 'status']
        };

        for (const [eventName, kinds] of Object.entries(map)) {
            es.addEventListener(eventName, () => {
                for (const k of kinds) scheduleReload(k);
            });
        }

        es.onerror = () => {
            eventStreamConnected = false;
            try {
                es.close();
            } catch {
                // ignore
            }
        };
    } catch {
        eventStreamConnected = false;
    }
}

function showToast(message, type = 'info', timeoutMs = 4500) {
    const container = document.getElementById('toast-container');
    if (!container) return;

    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    container.appendChild(toast);

    window.setTimeout(() => {
        toast.remove();
    }, timeoutMs);
}

function setBusy(key, isBusy) {
    if (isBusy) busy.add(key);
    else busy.delete(key);
    render();
}

function isBusy(key) {
    return busy.has(key);
}

async function loadBeads() {
    state.beads = await apiCall('/beads');
}

async function loadCeoBeads() {
    const ceoIds = getCeoAgentIds();
    if (ceoIds.length === 0) {
        state.ceoBeads = [];
        return;
    }
    const query = encodeURIComponent(ceoIds.join(','));
    state.ceoBeads = await apiCall(`/beads?assigned_to=${query}`);
}

async function loadAgents() {
    state.agents = await apiCall('/agents');
}

async function loadProviders() {
    state.providers = await apiCall('/providers');
}

async function loadProjects() {
    state.projects = await apiCall('/projects');
}

async function loadPersonas() {
    state.personas = await apiCall('/personas');
}

async function loadDecisions() {
    state.decisions = await apiCall('/decisions');
}

async function loadSystemStatus() {
    state.systemStatus = await apiCall('/system/status');
}

async function loadUsers() {
    if (!AUTH_ENABLED) { state.users = []; return; }
    try {
        state.users = await apiCall('/auth/users', { suppressToast: true, skipAutoFile: true });
    } catch (error) {
        // Users endpoint may not be available without auth
        state.users = [];
    }
}

async function loadAPIKeys() {
    if (!AUTH_ENABLED) { state.apiKeys = []; return; }
    try {
        state.apiKeys = await apiCall('/auth/api-keys', { suppressToast: true, skipAutoFile: true });
    } catch (error) {
        // API keys endpoint may not be available without auth
        state.apiKeys = [];
    }
}

// Render functions
function render() {
    if (modalState.activeId) return; // Skip render while a modal is open
    if (window.pairState && window.pairState.open) return; // Skip render while pair panel is open
    renderSystemStatus();
    renderProjectViewer();
    renderKanban();
    renderProviders();
    renderAgents();
    renderProjects();
    renderPersonas();
    renderDecisions();
    renderCeoDashboard();
    renderCeoBeads();
    renderUsers();
    renderDiagrams();

    // New UI components
    if (typeof renderProjectsTable === 'function') {
        renderProjectsTable();
    }
    if (typeof renderConversationsView === 'function') {
        renderConversationsView();
    }
}

function renderProjectViewer() {
    const select = document.getElementById('project-view-select');
    const details = document.getElementById('project-view-details');
    if (!select || !details) return;

    const projects = state.projects || [];
    if (projects.length === 0) {
        select.innerHTML = '';
        details.innerHTML = renderEmptyState('No projects configured', 'Add a project to start tracking beads and agents.');
        return;
    }

    if (!uiState.project.selectedId) {
        uiState.project.selectedId = projects[0].id;
    }

    select.innerHTML = projects
        .map((p) => `<option value="${escapeHtml(p.id)}" ${p.id === uiState.project.selectedId ? 'selected' : ''}>${escapeHtml(p.name)} (${escapeHtml(p.id)})</option>`)
        .join('');

    const project = projects.find((p) => p.id === uiState.project.selectedId) || projects[0];
    uiState.project.selectedId = project.id;

    // Get agents assigned to this project
    const projectAgents = (state.agents || []).filter((a) => a.project_id === project.id);

    // Status badge class
    const statusClass = project.status === 'closed' ? 'priority-3' : '';

    details.innerHTML = `
        <div class="project-header" style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; padding-bottom: 0.75rem; border-bottom: 1px solid var(--border-color);">
            <div>
                <span class="badge ${statusClass}" style="margin-right: 0.5rem;">${escapeHtml(project.status || 'open')}</span>
                <span class="small" style="color: var(--text-muted);">${escapeHtml(project.git_repo || '')} @ ${escapeHtml(project.branch || 'main')}</span>
            </div>
            <div style="display: flex; gap: 0.5rem;">
                <button type="button" class="secondary" onclick="showProjectSettingsModal('${escapeHtml(project.id)}')" title="Project Settings" style="padding: 0.5rem 0.75rem;">
                    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="vertical-align: middle;">
                        <circle cx="12" cy="12" r="3"></circle>
                        <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
                    </svg>
                </button>
                <button type="button" class="danger" onclick="deleteProject('${escapeHtml(project.id)}')" title="Delete Project" style="padding: 0.5rem 0.75rem;">
                    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="vertical-align: middle;">
                        <polyline points="3 6 5 6 21 6"></polyline>
                        <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
                    </svg>
                </button>
            </div>
        </div>
        <div style="display: grid; grid-template-columns: 1fr; gap: 1rem;">
            <div>
                <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem;">
                    <h4 style="color: var(--primary-color); margin: 0;">Assigned Agents (${projectAgents.length})</h4>
                    <button type="button" class="secondary" style="padding: 0.25rem 0.75rem; font-size: 0.85rem;" onclick="showAddAgentToProjectModal('${escapeHtml(project.id)}')">+ Add Agent</button>
                </div>
                <div id="project-agents-list" style="max-height: 200px; overflow-y: auto; border: 1px solid var(--border-color); border-radius: 4px;">
                    ${renderProjectAgentsList(projectAgents, project.id)}
                </div>
            </div>
        </div>
    `;

    const beads = (state.beads || []).filter((b) => b.project_id === project.id);
    const openBeads = beads.filter((b) => b.status === 'open');
    const inProgressBeads = beads.filter((b) => b.status === 'in_progress');
    const closedBeads = beads.filter((b) => b.status === 'closed');

    const openEl = document.getElementById('project-open-beads');
    const ipEl = document.getElementById('project-in-progress-beads');
    const closedEl = document.getElementById('project-closed-beads');
    if (openEl) {
        openEl.innerHTML = openBeads.length ? openBeads.map(renderBeadCard).join('') : renderEmptyState('No open beads', '');
    }
    if (ipEl) {
        ipEl.innerHTML = inProgressBeads.length ? inProgressBeads.map(renderBeadCard).join('') : renderEmptyState('Nothing in progress', '');
    }
    if (closedEl) {
        closedEl.innerHTML = closedBeads.length ? closedBeads.map(renderBeadCard).join('') : renderEmptyState('No closed beads', '');
    }

    // Keep the assignments board for backward compatibility
    const assignmentsEl = document.getElementById('project-agent-assignments');
    if (assignmentsEl) {
        assignmentsEl.style.display = 'none';
    }
}

// Org chart hierarchy order for sorting agents
const ORG_CHART_ORDER = [
    'ceo', 'cfo', 'product-manager', 'engineering-manager', 'project-manager',
    'qa-engineer', 'devops-engineer', 'code-reviewer', 'web-designer',
    'web-designer-engineer', 'documentation-manager', 'public-relations-manager',
    'decision-maker', 'housekeeping-bot'
];

function getOrgChartRank(agent) {
    const role = agent.role || extractRoleKey(agent.persona_name);
    const idx = ORG_CHART_ORDER.indexOf(role);
    return idx >= 0 ? idx : 999;
}

function extractRoleKey(personaName) {
    if (!personaName) return '';
    const parts = personaName.split('/');
    if (parts.length >= 2 && parts[0] === 'default') {
        return parts[1];
    } else if (parts.length >= 3 && parts[0] === 'projects') {
        return parts[2];
    }
    return parts[parts.length - 1];
}

function sortAgentsByOrgChart(agents) {
    return [...agents].sort((a, b) => getOrgChartRank(a) - getOrgChartRank(b));
}

function renderProjectAgentsList(agents, projectId) {
    if (agents.length === 0) {
        return `<div class="empty-state" style="padding: 1rem;"><p>No agents assigned yet.</p><p class="small">Add an agent from the org chart to get started.</p></div>`;
    }

    const sortedAgents = sortAgentsByOrgChart(agents);

    return sortedAgents.map((a) => {
        const bead = a.current_bead ? (state.beads || []).find((b) => b.id === a.current_bead) : null;
        const statusClass = a.status === 'working' ? 'working' : (a.status === 'blocked' ? 'blocked' : (a.status === 'paused' ? 'paused' : 'idle'));
        const roleName = extractRoleName(a.persona_name || a.name);
        
        return `
            <div class="agent-assignment-row" style="display: flex; justify-content: space-between; align-items: center; padding: 0.5rem; border-bottom: 1px solid var(--border-color); background: var(--card-bg);">
                <div style="flex: 1;">
                    <strong style="color: var(--text-color);">${escapeHtml(formatAgentDisplayName(a.name || roleName))}</strong>
                    <span class="badge ${statusClass}" style="margin-left: 0.5rem;">${escapeHtml(a.status || 'idle')}</span>
                    <div class="small" style="color: var(--text-muted);">
                        ${escapeHtml(a.persona_name || '')}
                        ${bead ? ` • Working on: ${escapeHtml(bead.title.substring(0, 30))}...` : ''}
                    </div>
                </div>
                <div style="display: flex; gap: 0.25rem;">
                    ${a.current_bead ? `<button type="button" class="secondary" style="padding: 0.25rem 0.5rem; font-size: 0.75rem;" onclick="viewAgentConversation('${escapeHtml(a.current_bead)}')" title="View conversation log">Log</button>` : ''}
                    <button type="button" class="secondary" style="padding: 0.25rem 0.5rem; font-size: 0.75rem;" onclick="showEditAgentModal('${escapeHtml(a.id)}')" title="Edit">Edit</button>
                    <button type="button" class="secondary" style="padding: 0.25rem 0.5rem; font-size: 0.75rem;" onclick="showCloneAgentModal('${escapeHtml(a.id)}')" title="Clone">Clone</button>
                    <button type="button" class="danger" style="padding: 0.25rem 0.5rem; font-size: 0.75rem;" onclick="removeAgentFromProject('${escapeHtml(projectId)}', '${escapeHtml(a.id)}')">Remove</button>
                </div>
            </div>
        `;
    }).join('');
}

function formatAgentDisplayName(name) {
    if (!name) return 'Agent';
    // Capitalize CEO and CFO as acronyms
    return name
        .replace(/\bCeo\b/gi, 'CEO')
        .replace(/\bCfo\b/gi, 'CFO');
}

function extractRoleName(personaName) {
    if (!personaName) return 'Agent';
    // Extract role from persona paths like "default/engineering-manager" or "projects/foo/engineering-manager/custom"
    const parts = personaName.split('/');
    let role = parts[parts.length - 1];
    if (parts.length >= 2 && parts[0] === 'default') {
        role = parts[1];
    } else if (parts.length >= 3 && parts[0] === 'projects') {
        role = parts[2];
    }
    // Convert kebab-case to Title Case, then fix acronyms
    const titleCase = role.split('-').map(w => w.charAt(0).toUpperCase() + w.slice(1)).join(' ');
    return formatAgentDisplayName(titleCase);
}

async function showAddAgentToProjectModal(projectId) {
    // Get available personas (the "org chart")
    const personas = state.personas || [];
    if (personas.length === 0) {
        showToast('No personas available. Add personas to the personas directory first.', 'error');
        return;
    }

    // Get providers
    const providers = state.providers || [];
    if (providers.length === 0) {
        showToast('No providers registered. Register a provider first.', 'error');
        return;
    }

    // Build persona options - only show default personas (the org chart), excluding templates
    const defaultPersonas = personas.filter(p => 
        p.name && 
        p.name.startsWith('default/') && 
        p.name !== 'templates'
    );
    const personaOptions = defaultPersonas.map(p => {
        const roleName = extractRoleName(p.name);
        return { value: p.name, label: `${roleName} (${p.name})` };
    });

    if (personaOptions.length === 0) {
        showToast('No default personas found in org chart. Add personas under personas/default/.', 'error');
        return;
    }

    // Build provider options
    const providerOptions = providers.map(p => ({
        value: p.id,
        label: `${p.name || p.id} (${p.status || 'unknown'})`
    }));

    try {
        const res = await formModal({
            title: 'Add Agent to Project',
            submitText: 'Create & Assign',
            fields: [
                {
                    id: 'persona_name',
                    label: 'Agent Role (from Org Chart)',
                    type: 'select',
                    required: true,
                    options: personaOptions
                },
                {
                    id: 'provider_id',
                    label: 'Provider',
                    type: 'select',
                    required: true,
                    options: providerOptions
                },
                {
                    id: 'custom_name',
                    label: 'Custom Name (optional)',
                    type: 'text',
                    required: false,
                    placeholder: 'Leave empty for default name'
                }
            ]
        });

        if (!res) return;

        // Create the agent and assign to project
        const roleName = extractRoleName(res.persona_name);
        const agentName = res.custom_name || `${roleName} (Default)`;

        setBusy('addAgentToProject', true);
        await apiCall('/agents', {
            method: 'POST',
            body: JSON.stringify({
                name: agentName,
                persona_name: res.persona_name,
                project_id: projectId,
                provider_id: res.provider_id
            })
        });

        showToast(`Agent "${agentName}" created and assigned`, 'success');
        await loadAll();
    } catch (error) {
        // Error already handled by apiCall
    } finally {
        setBusy('addAgentToProject', false);
    }
}

async function removeAgentFromProject(projectId, agentId) {
    const agent = (state.agents || []).find(a => a.id === agentId);
    const agentName = agent ? (agent.name || agentId) : agentId;

    const ok = await confirmModal({
        title: 'Remove Agent?',
        body: `Remove "${agentName}" from this project? This will stop the agent.`,
        confirmText: 'Remove',
        cancelText: 'Cancel',
        danger: true
    });

    if (!ok) return;

    try {
        setBusy(`removeAgent:${agentId}`, true);
        await apiCall(`/agents/${agentId}`, { method: 'DELETE' });
        showToast('Agent removed', 'success');
        await loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`removeAgent:${agentId}`, false);
    }
}

async function showEditAgentModal(agentId) {
    const agent = (state.agents || []).find(a => a.id === agentId);
    if (!agent) {
        showToast('Agent not found', 'error');
        return;
    }

    const persona = agent.persona || {};
    const displayName = formatAgentDisplayName(agent.name || extractRoleName(agent.persona_name));

    try {
        const res = await formModal({
            title: `Edit Agent: ${displayName}`,
            submitText: 'Save Changes',
            fields: [
                {
                    id: 'name',
                    label: 'Agent Name',
                    type: 'text',
                    required: true,
                    value: agent.name || ''
                },
                {
                    id: 'mission',
                    label: 'Mission (Job Description)',
                    type: 'textarea',
                    required: false,
                    value: persona.mission || '',
                    placeholder: 'Describe what this agent does...'
                },
                {
                    id: 'character',
                    label: 'Character',
                    type: 'textarea',
                    required: false,
                    value: persona.character || '',
                    placeholder: 'Describe the agent\'s character...'
                },
                {
                    id: 'tone',
                    label: 'Tone',
                    type: 'textarea',
                    required: false,
                    value: persona.tone || '',
                    placeholder: 'e.g., Professional, friendly, direct...'
                },
                {
                    id: 'autonomy_level',
                    label: 'Autonomy Level',
                    type: 'select',
                    required: true,
                    value: persona.autonomy_level || 'semi',
                    options: [
                        { value: 'full', label: 'Full - Can make all non-P0 decisions' },
                        { value: 'semi', label: 'Semi - Can make routine decisions' },
                        { value: 'supervised', label: 'Supervised - Requires approval for all decisions' }
                    ]
                }
            ]
        });

        if (!res) return;

        // Update agent persona via API
        setBusy(`editAgent:${agentId}`, true);
        await apiCall(`/agents/${agentId}`, {
            method: 'PUT',
            body: JSON.stringify({
                name: res.name,
                persona: {
                    ...persona,
                    mission: res.mission,
                    character: res.character,
                    tone: res.tone,
                    autonomy_level: res.autonomy_level
                }
            })
        });
        showToast('Agent updated', 'success');
        await loadAll();
    } catch (error) {
        // Error already handled by apiCall
    } finally {
        setBusy(`editAgent:${agentId}`, false);
    }
}

async function showCloneAgentModal(agentId) {
    const agent = (state.agents || []).find(a => a.id === agentId);
    if (!agent) {
        showToast('Agent not found', 'error');
        return;
    }

    const displayName = formatAgentDisplayName(agent.name || extractRoleName(agent.persona_name));
    const roleName = agent.role || extractRoleKey(agent.persona_name);

    try {
        const res = await formModal({
            title: `Clone Agent: ${displayName}`,
            submitText: 'Create Clone',
            fields: [
                {
                    id: 'new_name',
                    label: 'New Agent Name',
                    type: 'text',
                    required: true,
                    value: `${agent.name || roleName} (Copy)`,
                    placeholder: 'Enter name for the cloned agent'
                },
                {
                    id: 'new_persona_name',
                    label: 'New Persona Name',
                    type: 'text',
                    required: true,
                    value: `${roleName}-copy`,
                    placeholder: 'e.g., custom-reviewer'
                },
                {
                    id: 'replace_original',
                    label: 'Replace Original',
                    type: 'checkbox',
                    required: false,
                    value: false,
                    description: 'Stop the original agent after cloning'
                }
            ]
        });

        if (!res) return;

        setBusy(`cloneAgent:${agentId}`, true);
        await apiCall(`/agents/${agentId}/clone`, {
            method: 'POST',
            body: JSON.stringify({
                new_agent_name: res.new_name,
                new_persona_name: res.new_persona_name,
                replace: res.replace_original || false
            })
        });
        showToast('Agent cloned successfully', 'success');
        await loadAll();
    } catch (error) {
        // Error already handled by apiCall
    } finally {
        setBusy(`cloneAgent:${agentId}`, false);
    }
}

async function showProjectSettingsModal(projectId) {
    const project = state.projects.find((p) => p.id === projectId);
    if (!project) {
        showToast('Project not found', 'error');
        return;
    }

    try {
        const usesSSH = project.git_auth_method === 'ssh'
            || (project.git_repo || '').trim().startsWith('git@')
            || (project.git_repo || '').trim().startsWith('ssh://');
        let gitKeyValue = '';
        let gitKeyPlaceholder = 'Not available';
        let gitKeyDescription = 'SSH git auth is required to display the deploy key.';
        if (usesSSH) {
            try {
                const keyRes = await apiCall(`/projects/${projectId}/git-key`, {
                    suppressToast: true,
                    skipAutoFile: true
                });
                gitKeyValue = keyRes?.public_key || '';
                gitKeyPlaceholder = gitKeyValue ? '' : 'No key returned';
                gitKeyDescription = 'Add this as a write-enabled deploy key in your git host.';
            } catch (error) {
                gitKeyDescription = 'Unable to fetch the git key.';
            }
        }
        const res = await formModal({
            title: `Project Settings: ${project.name}`,
            submitText: 'Save Settings',
            fields: [
                {
                    id: 'name',
                    label: 'Project Name',
                    type: 'text',
                    required: true,
                    value: project.name || ''
                },
                {
                    id: 'git_repo',
                    label: 'GitHub Repository',
                    type: 'text',
                    required: true,
                    value: project.git_repo || '',
                    placeholder: 'https://github.com/org/repo'
                },
                {
                    id: 'git_key',
                    label: 'Project Git Key (Deploy Key)',
                    type: 'textarea',
                    required: false,
                    value: gitKeyValue,
                    placeholder: gitKeyPlaceholder,
                    description: gitKeyDescription,
                    readonly: true
                },
                {
                    id: 'branch',
                    label: 'Branch',
                    type: 'text',
                    required: true,
                    value: project.branch || 'main'
                },
                {
                    id: 'beads_path',
                    label: 'Beads Path',
                    type: 'text',
                    required: false,
                    value: project.beads_path || '.beads',
                    placeholder: '.beads'
                },
                {
                    id: 'status',
                    label: 'Status',
                    type: 'select',
                    required: true,
                    value: project.status || 'open',
                    options: [
                        { value: 'open', label: 'Open' },
                        { value: 'closed', label: 'Closed' },
                        { value: 'reopened', label: 'Reopened' }
                    ]
                },
                {
                    id: 'is_perpetual',
                    label: 'Perpetual Project',
                    type: 'select',
                    required: false,
                    value: project.is_perpetual ? 'true' : 'false',
                    options: [
                        { value: 'false', label: 'No - Project can be closed' },
                        { value: 'true', label: 'Yes - Project never closes' }
                    ]
                },
                {
                    id: 'is_sticky',
                    label: 'Sticky Project',
                    type: 'select',
                    required: false,
                    value: project.is_sticky ? 'true' : 'false',
                    options: [
                        { value: 'false', label: 'No' },
                        { value: 'true', label: 'Yes - Auto-added on startup' }
                    ]
                }
            ]
        });

        if (!res) return;

        const payload = {
            name: res.name,
            git_repo: res.git_repo,
            branch: res.branch,
            beads_path: res.beads_path || '.beads',
            status: res.status,
            is_perpetual: res.is_perpetual === 'true',
            is_sticky: res.is_sticky === 'true'
        };

        setBusy('projectSettings', true);
        await apiCall(`/projects/${projectId}`, {
            method: 'PUT',
            body: JSON.stringify(payload)
        });

        showToast('Project settings saved', 'success');
        await loadProjects();
        render();
    } catch (error) {
        // Error already handled by apiCall
    } finally {
        setBusy('projectSettings', false);
    }
}

async function assignAgentToProject(projectId) {
    try {
        const res = await formModal({
            title: 'Assign agent to project',
            submitText: 'Assign',
            fields: [{ id: 'agent_id', label: 'Agent ID', type: 'text', required: true, placeholder: 'agent-123' }]
        });
        if (!res) return;

        await apiCall(`/projects/${projectId}/agents`, {
            method: 'POST',
            body: JSON.stringify({ agent_id: res.agent_id, action: 'assign' })
        });

        showToast('Agent assigned', 'success');
        loadAll();
    } catch (error) {
        // Error already handled
    }
}

async function unassignAgentFromProject(projectId, agentId) {
    try {
        await apiCall(`/projects/${projectId}/agents`, {
            method: 'POST',
            body: JSON.stringify({ agent_id: agentId, action: 'unassign' })
        });

        showToast('Agent unassigned', 'success');
        loadAll();
    } catch (error) {
        // Error already handled
    }
}

function renderSystemStatus() {
    const el = document.getElementById('system-status');
    if (!el) return;

    const s = state.systemStatus;
    if (!s) {
        el.innerHTML = '';
        return;
    }

    const badge = s.state === 'active' ? `<span class="badge">active</span>` : `<span class="badge">parked</span>`;
    const reason = s.reason ? escapeHtml(s.reason) : '';
    el.innerHTML = `${badge} ${reason}`;
}

function renderKanban() {
    const filtered = getFilteredBeads();
    const openBeads = filtered.filter((b) => b.status === 'open');
    const inProgressBeads = filtered.filter((b) => b.status === 'in_progress');
    const closedBeads = filtered.filter((b) => b.status === 'closed');

    document.getElementById('open-beads').innerHTML =
        openBeads.length > 0
            ? openBeads.map(renderBeadCard).join('')
            : renderEmptyState('No open beads', 'Create a bead via the API or bd CLI, then it will show up here.');
    document.getElementById('in-progress-beads').innerHTML =
        inProgressBeads.length > 0
            ? inProgressBeads.map(renderBeadCard).join('')
            : renderEmptyState('Nothing in progress', 'Claim a bead to move it into progress.');
    document.getElementById('closed-beads').innerHTML =
        closedBeads.length > 0
            ? closedBeads.map(renderBeadCard).join('')
            : renderEmptyState('No closed beads yet', 'Completed beads will appear here.');
}

function initKanbanDnD() {
    const dropzones = document.querySelectorAll('.kanban-dropzone');
    dropzones.forEach((zone) => {
        zone.addEventListener('dragover', handleKanbanDragOver);
        zone.addEventListener('dragleave', handleKanbanDragLeave);
        zone.addEventListener('drop', handleKanbanDrop);
    });
}

function handleBeadDragStart(event) {
    const beadId = event.currentTarget?.dataset?.beadId;
    if (!beadId) return;
    event.dataTransfer.setData('text/plain', beadId);
    event.dataTransfer.effectAllowed = 'move';
    event.currentTarget.classList.add('dragging');
}

function handleBeadDragEnd(event) {
    event.currentTarget.classList.remove('dragging');
}

function handleKanbanDragOver(event) {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
    event.currentTarget.classList.add('drag-over');
}

function handleKanbanDragLeave(event) {
    event.currentTarget.classList.remove('drag-over');
}

async function handleKanbanDrop(event) {
    event.preventDefault();
    event.currentTarget.classList.remove('drag-over');
    const beadId = event.dataTransfer.getData('text/plain');
    const status = event.currentTarget.dataset.status;
    if (!beadId || !status) return;

    const bead = state.beads.find((b) => b.id === beadId);
    if (!bead || bead.status === status) return;

    try {
        await saveBeadUpdate(beadId, { status: status }, { successMessage: `Bead moved to ${status}` });
    } catch (error) {
        // Error already handled
    }
}

async function sendStreamingTest() {
    const providerSelect = document.getElementById('stream-test-provider');
    const input = document.getElementById('stream-test-input');
    const responseEl = document.getElementById('stream-test-response');
    const statsEl = document.getElementById('stream-test-stats');
    const sendBtn = document.getElementById('stream-test-send');
    const streamToggle = document.getElementById('stream-test-streaming');
    
    if (!providerSelect || !input || !responseEl || !sendBtn) return;

    const providerId = providerSelect.value;
    const message = (input.value || '').trim();
    const useStreaming = streamToggle ? streamToggle.checked : true;

    if (!providerId) {
        showToast('Please select a provider', 'error');
        return;
    }

    if (!message) {
        showToast('Please enter a message', 'error');
        return;
    }

    try {
        setBusy('streamTest', true);
        sendBtn.disabled = true;
        sendBtn.textContent = useStreaming ? 'Streaming…' : 'Sending…';
        responseEl.textContent = '';
        responseEl.classList.remove('complete');
        statsEl.textContent = 'Starting...';

        const startTime = Date.now();
        let firstChunkTime = null;
        let chunkCount = 0;
        let totalChars = 0;

        const requestBody = {
            provider_id: providerId,
            messages: [
                { role: 'user', content: message }
            ]
        };

        if (useStreaming) {
            // Use streaming
            responseEl.classList.add('streaming');
            
            await createStreamingRequest('/chat/completions', requestBody, {
                useStreaming: true,
                onChunk: (chunk, fullContent) => {
                    if (firstChunkTime === null) {
                        firstChunkTime = Date.now();
                    }
                    chunkCount++;
                    totalChars += chunk.length;
                    
                    responseEl.textContent = fullContent;
                    responseEl.scrollTop = responseEl.scrollHeight;
                    
                    const elapsed = Date.now() - startTime;
                    const ttfb = firstChunkTime - startTime;
                    statsEl.textContent = `Streaming... ${chunkCount} chunks, ${totalChars} chars, TTFB: ${ttfb}ms, Elapsed: ${elapsed}ms`;
                },
                onComplete: (fullContent) => {
                    const totalTime = Date.now() - startTime;
                    const ttfb = firstChunkTime ? firstChunkTime - startTime : 0;
                    const charsPerSec = totalTime > 0 ? Math.round((totalChars / totalTime) * 1000) : 0;
                    
                    responseEl.classList.remove('streaming');
                    responseEl.classList.add('complete');
                    statsEl.textContent = `✓ Complete: ${chunkCount} chunks, ${totalChars} chars, TTFB: ${ttfb}ms, Total: ${totalTime}ms, Speed: ${charsPerSec} chars/sec`;
                    showToast('Stream complete', 'success');
                },
                onError: (error) => {
                    responseEl.classList.remove('streaming');
                    responseEl.textContent += `\n\n[Error: ${error}]`;
                    statsEl.textContent = `✗ Error: ${error}`;
                }
            });
        } else {
            // Use non-streaming
            const res = await apiCall('/chat/completions', {
                method: 'POST',
                body: JSON.stringify(requestBody)
            });

            const totalTime = Date.now() - startTime;
            
            if (res.choices && res.choices[0] && res.choices[0].message) {
                const content = res.choices[0].message.content || 'No response';
                responseEl.textContent = content;
                totalChars = content.length;
            } else {
                responseEl.textContent = 'No response returned.';
            }
            
            responseEl.classList.add('complete');
            statsEl.textContent = `✓ Complete (non-streaming): ${totalChars} chars, Total: ${totalTime}ms`;
        }
    } catch (e) {
        responseEl.classList.remove('streaming');
        responseEl.textContent = `Request failed: ${e.message || 'Unknown error'}`;
        statsEl.textContent = `✗ Failed: ${e.message || 'Unknown error'}`;
    } finally {
        setBusy('streamTest', false);
        sendBtn.disabled = false;
        sendBtn.textContent = 'Send';
    }
}

const OPENAI_COMPATIBLE_TYPES = new Set(['openai', 'local', 'custom', 'anthropic', 'vllm']);

function parseProviderEndpoint(endpoint) {
    if (!endpoint) return null;
    try {
        return new URL(endpoint);
    } catch (err) {
        try {
            return new URL(`http://${endpoint}`);
        } catch (err2) {
            return null;
        }
    }
}

function looksLikeOllamaEndpoint(endpoint) {
    const url = parseProviderEndpoint(endpoint);
    if (!url) return false;
    const port = url.port;
    const path = url.pathname || '';
    return port === '11434' || path.startsWith('/api') || path.includes('/api/');
}

function looksLikeOpenAIEndpoint(endpoint) {
    const url = parseProviderEndpoint(endpoint);
    if (!url) return false;
    const path = url.pathname || '';
    return path.startsWith('/v1') || path.includes('/v1/');
}

function getProviderEndpointWarning(provider) {
    if (!provider) return '';
    const endpoint = provider.endpoint || '';
    const type = String(provider.type || '').toLowerCase();
    if (!endpoint || !type) return '';

    if (type === 'ollama' && looksLikeOpenAIEndpoint(endpoint)) {
        return 'This endpoint looks OpenAI-compatible (/v1) but the protocol is Ollama.';
    }
    if (OPENAI_COMPATIBLE_TYPES.has(type) && looksLikeOllamaEndpoint(endpoint)) {
        return 'This endpoint looks like an Ollama server (port 11434 or /api), but the protocol is OpenAI-compatible.';
    }
    return '';
}

function renderProviders() {
    const container = document.getElementById('provider-list');
    if (!container) return;
    
    // Also populate streaming test provider dropdown
    const streamTestSelect = document.getElementById('stream-test-provider');
    if (streamTestSelect && state.providers) {
        // Show ALL providers in streaming test (including failed ones for testing)
        streamTestSelect.innerHTML = 
            '<option value="">Select a provider...</option>' +
            state.providers.map(p => {
                const statusBadge = p.status === 'active' ? '' : ` [${p.status}]`;
                return `<option value="${escapeHtml(p.id)}">${escapeHtml(p.name || p.id)} (${escapeHtml(p.model || 'unknown')})${statusBadge}</option>`;
            }).join('');
    }

    if (!state.providers || state.providers.length === 0) {
        container.innerHTML = renderEmptyState(
            'No providers registered',
            'Register at least one vLLM/OpenAI-compatible provider to enable agent execution.',
            '<button type="button" onclick="showRegisterProviderModal()">Register Provider</button>'
        );
        return;
    }

    container.innerHTML = state.providers
        .map((p) => {
            const id = escapeHtml(p.id || '');
            const name = escapeHtml(p.name || p.id || '');
            const endpoint = escapeHtml(p.endpoint || '');
            const endpointWarning = getProviderEndpointWarning(p);
            const model = escapeHtml(p.model || '');
            const configuredModel = escapeHtml(p.configured_model || p.model || '');
            const selectedModel = escapeHtml(p.selected_model || p.model || '');
            const selectionReason = escapeHtml(p.selection_reason || '');
            const modelScore = p.model_score ?? null;
            const selectedGpu = escapeHtml(p.selected_gpu || '');
            const status = escapeHtml(p.status || 'unknown');
            const heartbeatLatency = p.last_heartbeat_latency_ms ?? null;
            const heartbeatError = escapeHtml(p.last_heartbeat_error || '');
            const hasApiKey = p.key_id ? '*'.repeat(8) : '<em>none</em>';
            const modelsKey = `providerModels:${id}`;
            const deleteKey = `deleteProvider:${id}`;
            const negotiateKey = `providerNegotiate:${id}`;

            return `
                <div class="provider-card">
                    <div class="provider-header">
                        <div><strong>${name}</strong><div class="small">${id}</div></div>
                        <div><span class="badge">${escapeHtml(p.type || '')}</span></div>
                    </div>
                    <div class="small"><strong>Endpoint:</strong> ${endpoint}</div>
                    ${endpointWarning ? `<div class="small" style="color: var(--warning-color);"><strong>Warning:</strong> ${escapeHtml(endpointWarning)}</div>` : ''}
                    <div class="small"><strong>API Key:</strong> ${hasApiKey}</div>
                    <div class="small"><strong>Configured model:</strong> ${configuredModel || '<em>unset</em>'}</div>
                    <div class="small"><strong>Selected model:</strong> ${selectedModel || '<em>unset</em>'}</div>
                    <div class="small"><strong>Selection reason:</strong> ${selectionReason || '<em>pending</em>'}</div>
                    <div class="small"><strong>Model score:</strong> ${modelScore !== null ? escapeHtml(modelScore.toFixed(2)) : '<em>n/a</em>'}</div>
                    <div class="small"><strong>Selected GPU:</strong> ${selectedGpu || '<em>n/a</em>'}</div>
                    <div class="small"><strong>Status:</strong> ${status}</div>
                    <div class="small"><strong>Heartbeat latency:</strong> ${heartbeatLatency !== null && heartbeatLatency !== 0 ? `${escapeHtml(String(heartbeatLatency))}ms` : '<em>n/a</em>'}</div>
                    ${heartbeatError ? `<div class="small"><strong>Heartbeat error:</strong> ${heartbeatError}</div>` : ''}
                    <div class="provider-actions">
                        <button type="button" class="secondary" onclick="fetchProviderModels('${id}')" ${isBusy(modelsKey) ? 'disabled' : ''}>${isBusy(modelsKey) ? 'Loading…' : 'Models'}</button>
                        <button type="button" class="secondary" onclick="renegotiateProvider('${id}')" ${isBusy(negotiateKey) ? 'disabled' : ''}>${isBusy(negotiateKey) ? 'Negotiating…' : 'Re-negotiate model'}</button>
                        <button type="button" class="secondary" onclick="showEditProviderModal('${id}')">Edit</button>
                        <button type="button" class="secondary" onclick="deleteProvider('${id}')" ${isBusy(deleteKey) ? 'disabled' : ''}>${isBusy(deleteKey) ? 'Deleting…' : 'Delete'}</button>
                    </div>
                </div>
            `;
        })
        .join('');
}

function getFilteredBeads() {
    const q = (uiState.bead.search || '').trim().toLowerCase();
    const priority = uiState.bead.priority;
    const type = (uiState.bead.type || '').trim();
    const assigned = (uiState.bead.assigned || '').trim().toLowerCase();
    const tag = (uiState.bead.tag || '').trim().toLowerCase();

    const filtered = state.beads.filter((b) => {
        if (priority !== 'all' && String(b.priority) !== priority) return false;
        if (type !== 'all' && (b.type || '') !== type) return false;
        if (assigned && !(b.assigned_to || '').toLowerCase().includes(assigned)) return false;
        if (tag) {
            const tags = Array.isArray(b.tags) ? b.tags : [];
            if (!tags.some((t) => String(t).toLowerCase().includes(tag))) return false;
        }
        if (q) {
            const hay = `${b.id || ''} ${b.title || ''} ${b.description || ''}`.toLowerCase();
            if (!hay.includes(q)) return false;
        }
        return true;
    });

    const sort = uiState.bead.sort;
    filtered.sort((a, b) => {
        if (sort === 'title') return String(a.title || '').localeCompare(String(b.title || ''));
        if (sort === 'updated_at') {
            return String(b.updated_at || '').localeCompare(String(a.updated_at || ''));
        }
        // priority default
        return (a.priority ?? 99) - (b.priority ?? 99);
    });

    return filtered;
}

function renderEmptyState(title, description, actionsHtml = '') {
    return `
        <div class="empty-state" role="note">
            <h4>${escapeHtml(title)}</h4>
            <p>${escapeHtml(description)}</p>
            ${actionsHtml}
        </div>
    `;
}

function renderBeadCard(bead) {
    const priorityClass = `priority-${bead.priority}`;
    const typeClass = bead.type === 'decision' ? 'decision' : '';

    return `
        <button type="button" class="bead-card ${priorityClass} ${typeClass}" draggable="true" data-bead-id="${escapeHtml(bead.id)}" ondragstart="handleBeadDragStart(event)" ondragend="handleBeadDragEnd(event)" onclick="viewBead('${bead.id}')" ondblclick="openBeadDetails('${bead.id}')" aria-label="View bead: ${escapeHtml(bead.title)}">
            <div class="bead-title">${escapeHtml(bead.title)}</div>
            <div class="bead-meta">
                <span class="badge priority-${bead.priority}">P${bead.priority}</span>
                <span class="badge">${escapeHtml(bead.type)}</span>
                ${bead.assigned_to ? `<span class="badge">👤 ${escapeHtml(bead.assigned_to.substring(0, 8))}</span>` : '<span class="badge">unassigned</span>'}
            </div>
        </button>
    `;
}

function updateBeadCache(updatedBead) {
    if (!updatedBead || !updatedBead.id) return;
    const applyUpdate = (list) => {
        if (!Array.isArray(list)) return list;
        return list.map((bead) => (bead.id === updatedBead.id ? { ...bead, ...updatedBead } : bead));
    };
    state.beads = applyUpdate(state.beads);
    if (Array.isArray(state.ceoBeads)) {
        state.ceoBeads = applyUpdate(state.ceoBeads);
    }
}

async function saveBeadUpdate(beadId, payload, { successMessage = 'Bead updated' } = {}) {
    const existing = state.beads.find((b) => b.id === beadId);
    const previous = existing ? JSON.parse(JSON.stringify(existing)) : null;
    if (existing) {
        updateBeadCache({ ...existing, ...payload });
        render();
    }

    try {
        const updated = await apiCall(`/beads/${beadId}`, {
            method: 'PATCH',
            body: JSON.stringify(payload)
        });
        updateBeadCache(updated);
        render();
        showToast(successMessage, 'success');
        return updated;
    } catch (error) {
        if (previous) {
            updateBeadCache(previous);
            render();
        }
        throw error;
    }
}

function normalizeAssignee(value) {
    return String(value || '').trim().toLowerCase();
}

function isCeoAssignee(value) {
    const normalized = normalizeAssignee(value);
    return normalized && (normalized === 'ceo' || normalized === 'user-ceo' || normalized === 'human-ceo' || normalized.includes('ceo'));
}

function getCeoAgentIds() {
    return (state.agents || [])
        .filter((agent) => {
            const persona = normalizeAssignee(agent.persona_name);
            const name = normalizeAssignee(agent.name);
            const role = normalizeAssignee(agent.role);
            return persona.endsWith('/ceo') || persona === 'ceo' || role === 'ceo' || name.includes('ceo');
        })
        .map((agent) => normalizeAssignee(agent.id));
}

function parseBeadComments(context) {
    if (!context || !context.comments) return [];
    try {
        const parsed = JSON.parse(context.comments);
        return Array.isArray(parsed) ? parsed : [];
    } catch {
        return [];
    }
}

function getBeadCommentCount(bead) {
    return parseBeadComments(bead.context || {}).length;
}

function getLatestBeadComment(bead) {
    const comments = parseBeadComments(bead.context || {});
    if (comments.length === 0) return null;
    return comments[comments.length - 1];
}

function renderCeoBeads() {
    const container = document.getElementById('ceo-assigned-beads') || document.getElementById('ceo-assigned-beads-unified');
    if (!container) return;

    // Also populate the unified CEO section if it exists
    const unifiedContainer = document.getElementById('ceo-assigned-beads-unified');
    if (unifiedContainer && unifiedContainer !== container) {
        // Will be populated below after rendering
    }

    const ceoAgentIds = new Set(getCeoAgentIds());
    const sourceBeads = Array.isArray(state.ceoBeads) ? state.ceoBeads : state.beads;
    const beads = (sourceBeads || []).filter((bead) => {
        const assigned = normalizeAssignee(bead.assigned_to);
        if (!assigned) return false;
        if (ceoAgentIds.size > 0) {
            return ceoAgentIds.has(assigned);
        }
        return isCeoAssignee(assigned);
    });
    if (beads.length === 0) {
        container.innerHTML = renderEmptyState('No beads assigned to the CEO', 'Escalated or assigned beads will appear here.');
        return;
    }

    const agents = (state.agents || []).filter((a) => (a.persona_name || '') !== 'templates');
    const agentOptions = agents
        .map((agent) => {
            const displayName = formatAgentDisplayName(agent.name || extractRoleName(agent.persona_name || ''));
            return `<option value="${escapeHtml(agent.id)}">${escapeHtml(displayName)} (${escapeHtml(agent.id)})</option>`;
        })
        .join('');

    container.innerHTML = beads
        .map((bead) => {
            const commentCount = getBeadCommentCount(bead);
            const latestComment = getLatestBeadComment(bead);
            const dispatchKey = `dispatchBead:${bead.id}`;
            const closeKey = `closeBead:${bead.id}`;
            const commentKey = `commentBead:${bead.id}`;

            return `
                <div class="ceo-bead-card">
                    <div class="ceo-bead-header">
                        <button type="button" class="ceo-bead-title" onclick="viewBead('${escapeHtml(bead.id)}')" aria-label="View bead ${escapeHtml(bead.title)}">
                            ${escapeHtml(bead.title)}
                        </button>
                        <div class="ceo-bead-meta">
                            <span class="badge priority-${bead.priority}">P${bead.priority}</span>
                            <span class="badge">${escapeHtml(bead.type || 'task')}</span>
                            <span class="badge">${escapeHtml(bead.status || 'open')}</span>
                            <span class="badge">${commentCount} comment${commentCount === 1 ? '' : 's'}</span>
                        </div>
                    </div>
                    <div class="small"><strong>ID:</strong> ${escapeHtml(bead.id)} • <strong>Project:</strong> ${escapeHtml(bead.project_id || 'n/a')}</div>
                    ${latestComment ? `
                        <div class="ceo-bead-comment">
                            <div class="small"><strong>Latest comment</strong> · ${escapeHtml(latestComment.author || 'unknown')} · ${escapeHtml((latestComment.timestamp || '').substring(0, 19).replace('T', ' '))}</div>
                            <div class="small ceo-bead-comment-text">${escapeHtml(latestComment.comment || '')}</div>
                        </div>
                    ` : ''}
                    <div class="ceo-bead-actions">
                        <select id="ceo-dispatch-${escapeHtml(bead.id)}" aria-label="Select agent to dispatch bead ${escapeHtml(bead.id)}">
                            <option value="">Select agent…</option>
                            ${agentOptions}
                        </select>
                        <button type="button" class="secondary" onclick="dispatchBeadToAgent('${escapeHtml(bead.id)}')" ${isBusy(dispatchKey) ? 'disabled' : ''}>
                            ${isBusy(dispatchKey) ? 'Dispatching…' : 'Dispatch'}
                        </button>
                        <button type="button" class="secondary" onclick="showAllBeadCommentsModal('${escapeHtml(bead.id)}')">
                            View all comments
                        </button>
                        <button type="button" class="secondary" onclick="showBeadCommentModal('${escapeHtml(bead.id)}')" ${isBusy(commentKey) ? 'disabled' : ''}>
                            ${isBusy(commentKey) ? 'Saving…' : 'Add comment'}
                        </button>
                        <button type="button" class="danger" onclick="closeBeadFromCeo('${escapeHtml(bead.id)}')" ${isBusy(closeKey) ? 'disabled' : ''}>
                            ${isBusy(closeKey) ? 'Closing…' : 'Close'}
                        </button>
                    </div>
                </div>
            `;
        })
        .join('');
}

function renderAgents() {
    const q = (uiState.agent.search || '').trim().toLowerCase();
    // Filter out templates agent (it does nothing for now)
    const visibleAgents = state.agents.filter((a) => a.persona_name !== 'templates');
    const agents = q
        ? visibleAgents.filter((a) => {
              const hay = `${a.name || ''} ${a.persona_name || ''}`.toLowerCase();
              return hay.includes(q);
          })
        : visibleAgents;

    const html = agents.map(agent => {
        const statusClass = agent.status;
        return `
            <div class="agent-card ${statusClass}">
                <div class="agent-header">
                    <span class="agent-name">${escapeHtml(agent.name)}</span>
                    <span class="agent-status ${statusClass}">${agent.status}</span>
                </div>
                <div>
                    <strong>Persona:</strong> ${escapeHtml(agent.persona_name)}<br>
                    <strong>Project:</strong> ${agent.project_id.substring(0, 12)}...<br>
                    ${agent.current_bead ? `<strong>Working on:</strong> ${agent.current_bead}` : ''}
                </div>
                <div style="margin-top: 1rem;">
                    ${agent.current_bead ? `<button class="secondary" onclick="viewAgentConversation('${escapeHtml(agent.current_bead)}')" title="View conversation log">Log</button>` : ''}
                    <button class="secondary" onclick="cloneAgentPersona('${agent.id}')" ${isBusy(`cloneAgent:${agent.id}`) ? 'disabled' : ''}>${isBusy(`cloneAgent:${agent.id}`) ? 'Cloning…' : 'Clone Persona'}</button>
                    <button class="danger" onclick="stopAgent('${agent.id}')" ${isBusy(`stopAgent:${agent.id}`) ? 'disabled' : ''}>${isBusy(`stopAgent:${agent.id}`) ? 'Stopping…' : 'Stop Agent'}</button>
                </div>
            </div>
        `;
    }).join('');

    document.getElementById('agent-list').innerHTML =
        agents.length > 0
            ? html
            : renderEmptyState(
                  'No active agents',
                  'Spawn an agent to start working on beads.',
                  '<button type="button" class="secondary" onclick="showSpawnAgentModal()">Spawn your first agent</button>'
              );
}

function renderProjects() {
    const html = state.projects.map(project => `
        <div class="project-card">
            <h3>${escapeHtml(project.name)}</h3>
            <div>
                <strong>Branch:</strong> ${escapeHtml(project.branch)}<br>
                <strong>Repo:</strong> ${escapeHtml(project.git_repo)}<br>
                <strong>Agents:</strong> ${project.agents ? project.agents.length : 0}
            </div>
            <div style="margin-top: 0.75rem; display: flex; gap: 0.5rem; flex-wrap: wrap;">
                <button type="button" class="secondary" onclick="viewProject('${escapeHtml(project.id)}')">View</button>
                <button type="button" class="secondary" onclick="showEditProjectModal('${escapeHtml(project.id)}')">Edit</button>
                <button type="button" class="danger" onclick="deleteProject('${escapeHtml(project.id)}')">Delete</button>
            </div>
        </div>
    `).join('');
    
    document.getElementById('project-list').innerHTML =
        html || renderEmptyState('No projects configured', 'Add a project to get started.', '<button type="button" onclick="showCreateProjectModal()">Add Project</button>');
}

function viewProject(projectId) {
    uiState.project.selectedId = projectId;
    location.hash = '#project-viewer';
    render();
}

function projectFormFields(project = {}) {
    return [
        { id: 'name', label: 'Name', type: 'text', required: true, value: project.name || '' },
        { id: 'git_repo', label: 'Git repo', type: 'text', required: true, value: project.git_repo || '' },
        { id: 'branch', label: 'Branch', type: 'text', required: true, value: project.branch || 'main' },
        { id: 'beads_path', label: 'Beads path', type: 'text', required: false, value: project.beads_path || '.beads' },
        {
            id: 'is_perpetual',
            label: 'Perpetual project',
            type: 'select',
            required: false,
            value: project.is_perpetual ? 'true' : 'false',
            options: [
                { value: 'false', label: 'No' },
                { value: 'true', label: 'Yes' }
            ]
        },
        {
            id: 'is_sticky',
            label: 'Sticky project',
            type: 'select',
            required: false,
            value: project.is_sticky ? 'true' : 'false',
            options: [
                { value: 'false', label: 'No' },
                { value: 'true', label: 'Yes' }
            ]
        },
        {
            id: 'git_strategy',
            label: 'Git strategy',
            type: 'select',
            value: project.git_strategy || 'direct',
            options: [
                { value: 'direct', label: 'Direct to branch' },
                { value: 'branch-pr', label: 'Feature branch + PR' }
            ]
        }
    ];
}

function parseBool(value) {
    return value === 'true' || value === '1' || value === 'yes';
}

function buildProjectPayload(data) {
    const payload = {
        name: (data.name || '').trim(),
        git_repo: (data.git_repo || '').trim(),
        branch: (data.branch || '').trim(),
        beads_path: (data.beads_path || '').trim(),
        is_perpetual: parseBool(data.is_perpetual || 'false'),
        is_sticky: parseBool(data.is_sticky || 'false'),
        git_strategy: (data.git_strategy || 'direct').trim()
    };

    if (!payload.name) delete payload.name;
    if (!payload.git_repo) delete payload.git_repo;
    if (!payload.branch) delete payload.branch;
    if (!payload.beads_path) delete payload.beads_path;

    return payload;
}

async function showCreateProjectModal() {
    try {
        const res = await formModal({
            title: 'Add project',
            submitText: 'Create',
            fields: projectFormFields()
        });
        if (!res) return;

        await apiCall('/projects', {
            method: 'POST',
            body: JSON.stringify(buildProjectPayload(res))
        });

        showToast('Project created', 'success');
        await loadProjects();
        render();
    } catch (e) {
        // handled
    }
}

async function showEditProjectModal(projectId) {
    const project = state.projects.find((p) => p.id === projectId);
    if (!project) return;

    try {
        const res = await formModal({
            title: `Edit project ${project.name}`,
            submitText: 'Save',
            fields: projectFormFields(project)
        });
        if (!res) return;

        await apiCall(`/projects/${projectId}`, {
            method: 'PUT',
            body: JSON.stringify(buildProjectPayload(res))
        });

        showToast('Project updated', 'success');
        await loadProjects();
        render();
    } catch (e) {
        // handled
    }
}

async function deleteProject(projectId) {
    const project = state.projects.find((p) => p.id === projectId);
    const ok = await confirmModal({
        title: 'Delete project?',
        body: `This will delete project ${project ? project.name : projectId}.`,
        confirmText: 'Delete',
        cancelText: 'Cancel',
        danger: true
    });
    if (!ok) return;

    try {
        await apiCall(`/projects/${projectId}`, { method: 'DELETE' });
        showToast('Project deleted', 'success');
        await loadProjects();
        if (uiState.project.selectedId === projectId) {
            uiState.project.selectedId = '';
        }
        render();
    } catch (e) {
        // handled
    }
}

// Streaming utilities
function createStreamingRequest(endpoint, body, options = {}) {
    const { onChunk, onComplete, onError, useStreaming = true } = options;
    
    if (!useStreaming || typeof EventSource === 'undefined') {
        // Fallback to non-streaming
        return apiCall(endpoint, {
            method: 'POST',
            body: JSON.stringify(body)
        });
    }

    return new Promise((resolve, reject) => {
        // Use streaming endpoint
        const streamEndpoint = endpoint + '/stream';
        
        fetch(`${API_BASE}${streamEndpoint}`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                ...(authToken ? { 'Authorization': `Bearer ${authToken}` } : {})
            },
            body: JSON.stringify(body)
        }).then(response => {
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let buffer = '';
            let fullContent = '';

            function processText(text) {
                buffer += text;
                const lines = buffer.split('\n');
                buffer = lines.pop() || ''; // Keep incomplete line in buffer

                for (const line of lines) {
                    if (!line.trim()) continue;
                    
                    if (line.startsWith('event: ')) {
                        const eventType = line.substring(7).trim();
                        continue; // Event type will be followed by data line
                    }

                    if (line.startsWith('data: ')) {
                        const data = line.substring(6);
                        
                        try {
                            const chunk = JSON.parse(data);
                            
                            // Handle different event types
                            if (chunk.message) {
                                // Connected or done event
                                if (chunk.message.includes('complete')) {
                                    if (onComplete) onComplete(fullContent);
                                    resolve({ response: fullContent });
                                }
                                continue;
                            }

                            if (chunk.error) {
                                if (onError) onError(chunk.error);
                                reject(new Error(chunk.error));
                                return;
                            }

                            // Extract content from streaming chunk
                            if (chunk.choices && chunk.choices[0]) {
                                const delta = chunk.choices[0].delta;
                                if (delta && delta.content) {
                                    fullContent += delta.content;
                                    if (onChunk) onChunk(delta.content, fullContent);
                                }
                            }
                        } catch (e) {
                            // Ignore JSON parse errors for non-JSON data lines
                        }
                    }
                }
            }

            function pump() {
                reader.read().then(({ done, value }) => {
                    if (done) {
                        if (onComplete) onComplete(fullContent);
                        resolve({ response: fullContent });
                        return;
                    }

                    processText(decoder.decode(value, { stream: true }));
                    pump();
                }).catch(error => {
                    // Better error message for mid-stream failures
                    let errorMsg = error.message || 'Stream interrupted';
                    if (errorMsg.includes('network') || errorMsg.includes('NetworkError')) {
                        errorMsg = fullContent 
                            ? 'Connection lost during response (partial content received)' 
                            : 'Network error - check provider connection';
                    }
                    if (onError) onError(errorMsg);
                    reject(new Error(errorMsg));
                });
            }

            pump();
        }).catch(error => {
            // Provide more helpful error messages for common failures
            let errorMsg = error.message || 'Unknown error';
            if (errorMsg.includes('Failed to fetch') || errorMsg.includes('NetworkError')) {
                errorMsg = 'network error';
            } else if (errorMsg.includes('aborted')) {
                errorMsg = 'request was cancelled';
            }
            if (onError) onError(errorMsg);
            reject(new Error(errorMsg));
        });
    });
}

async function sendReplQuery() {
    const input = document.getElementById('repl-input');
    const responseEl = document.getElementById('repl-response');
    const sendBtn = document.getElementById('repl-send');
    const streamToggle = document.getElementById('repl-stream-toggle');
    if (!input || !responseEl || !sendBtn) return;

    const message = (input.value || '').trim();
    if (!message) {
        showToast('Enter a question first.', 'error');
        return;
    }

    const useStreaming = streamToggle ? streamToggle.checked : true;

    try {
        setBusy('repl', true);
        sendBtn.disabled = true;
        sendBtn.textContent = useStreaming ? 'Streaming…' : 'Sending…';
        responseEl.textContent = '';
        responseEl.classList.add('streaming');

        // Get healthy provider for the request
        const healthyProviders = (state.providers || []).filter(p => p.status === 'healthy' || p.status === 'active');
        if (healthyProviders.length === 0) {
            throw new Error('No active providers available');
        }
        const providerId = healthyProviders[0].id;

        const requestBody = {
            provider_id: providerId,
            messages: [
                { role: 'system', content: 'You are the CEO of Loom. Respond concisely and helpfully.' },
                { role: 'user', content: message }
            ]
        };

        if (useStreaming) {
            // Use streaming
            await createStreamingRequest('/chat/completions', requestBody, {
                useStreaming: true,
                onChunk: (chunk, fullContent) => {
                    responseEl.textContent = fullContent;
                    // Auto-scroll to bottom
                    responseEl.scrollTop = responseEl.scrollHeight;
                },
                onComplete: (fullContent) => {
                    responseEl.classList.remove('streaming');
                    responseEl.classList.add('complete');
                },
                onError: (error) => {
                    responseEl.classList.remove('streaming');
                    responseEl.textContent += `\n\n[Error: ${error}]`;
                }
            });
        } else {
            // Use non-streaming
            const res = await apiCall('/chat/completions', {
                method: 'POST',
                body: JSON.stringify(requestBody)
            });

            if (res.choices && res.choices[0] && res.choices[0].message) {
                responseEl.textContent = res.choices[0].message.content || 'No response';
            } else {
                responseEl.textContent = 'No response returned.';
            }
            responseEl.classList.remove('streaming');
            responseEl.classList.add('complete');
        }
    } catch (e) {
        responseEl.classList.remove('streaming');
        responseEl.textContent = `Request failed: ${e.message || 'Unknown error'}`;
    } finally {
        setBusy('repl', false);
        sendBtn.disabled = false;
        sendBtn.textContent = 'Send';
    }
}

// CEO REPL — supports "agent: message" routing and general queries
async function sendCeoReplQuery() {
    const input = document.getElementById('ceo-repl-input');
    const responseEl = document.getElementById('ceo-repl-response');
    const sendBtn = document.getElementById('ceo-repl-send');
    if (!input || !responseEl || !sendBtn) return;

    const message = (input.value || '').trim();
    if (!message) {
        showToast('Enter a question first.', 'error');
        return;
    }

    // Parse "agent: message" pattern for direct agent dispatch
    const agentMatch = message.match(/^([a-zA-Z][a-zA-Z0-9 _-]+):\s+(.+)/s);
    if (agentMatch) {
        const agentQuery = agentMatch[1].trim().toLowerCase();
        const taskMessage = agentMatch[2].trim();
        return ceoReplDispatchToAgent(agentQuery, taskMessage, responseEl, sendBtn);
    }

    // General query — use streaming chat completion
    return ceoReplStreamQuery(message, responseEl, sendBtn);
}

// Dispatch a task to a specific agent via "agent: message" syntax
async function ceoReplDispatchToAgent(agentQuery, taskMessage, responseEl, sendBtn) {
    const projectId = uiState.project.selectedId || ((state.projects || [])[0] || {}).id || '';
    if (!projectId) {
        responseEl.innerHTML = '<span style="color:var(--danger-color)">No project selected. Select a project first.</span>';
        return;
    }

    // Find matching agent in the current project
    const projectAgents = (state.agents || []).filter(a => a.project_id === projectId);
    const matchedAgent = projectAgents.find(a => {
        const name = (a.name || '').toLowerCase();
        const role = (a.role || '').toLowerCase();
        const persona = (a.persona_name || '').toLowerCase();
        return name.includes(agentQuery) || role.includes(agentQuery) || persona.includes(agentQuery);
    });

    if (!matchedAgent) {
        const available = projectAgents.map(a => {
            const display = a.name || a.role || a.persona_name;
            return `  - ${display} (${a.status})`;
        }).join('\n');
        responseEl.innerHTML = `<span style="color:var(--danger-color)">No agent matching "${escapeHtml(agentQuery)}" found in this project.</span>\n\n<strong>Available agents:</strong>\n${escapeHtml(available || '  (none)')}`;
        return;
    }

    try {
        setBusy('ceo-repl', true);
        sendBtn.disabled = true;
        sendBtn.textContent = 'Dispatching…';
        responseEl.textContent = '';

        // Create a bead with the task
        const bead = await apiCall('/beads', {
            method: 'POST',
            skipAutoFile: true,
            body: JSON.stringify({
                title: taskMessage.substring(0, 100),
                description: taskMessage,
                type: 'task',
                priority: 1,
                project_id: projectId
            })
        });

        // Assign to the matched agent
        await apiCall(`/beads/${bead.id}`, {
            method: 'PATCH',
            skipAutoFile: true,
            body: JSON.stringify({ assigned_to: matchedAgent.id })
        });

        const agentDisplay = matchedAgent.name || matchedAgent.role || matchedAgent.persona_name;
        responseEl.innerHTML = `<strong style="color:var(--success-color)">Task dispatched!</strong>\n\nBead: <code>${escapeHtml(bead.id)}</code>\nAssigned to: <strong>${escapeHtml(agentDisplay)}</strong>\nTitle: ${escapeHtml(bead.title)}\n\nThe dispatcher will pick this up on the next cycle.`;

        // Refresh data
        setTimeout(() => loadAll(), 1000);
    } catch (e) {
        responseEl.innerHTML = `<span style="color:var(--danger-color)">Failed to dispatch: ${escapeHtml(e.message)}</span>`;
    } finally {
        setBusy('ceo-repl', false);
        sendBtn.disabled = false;
        sendBtn.textContent = 'Send';
    }
}

// Stream a general CEO query via chat completion
async function ceoReplStreamQuery(message, responseEl, sendBtn) {
    try {
        setBusy('ceo-repl', true);
        sendBtn.disabled = true;
        sendBtn.textContent = 'Streaming…';
        responseEl.textContent = '';
        responseEl.classList.add('streaming');

        const healthyProviders = (state.providers || []).filter(p => p.status === 'healthy' || p.status === 'active');
        if (healthyProviders.length === 0) {
            throw new Error('No active providers available.');
        }
        const providerId = healthyProviders[0].id;

        // Build context about current project state
        const projectId = uiState.project.selectedId || ((state.projects || [])[0] || {}).id || '';
        const project = state.projects.find(p => p.id === projectId);
        const projectAgents = (state.agents || []).filter(a => a.project_id === projectId);
        const agentSummary = projectAgents.map(a => `${a.name || a.role} (${a.status}${a.current_bead ? ', working on ' + a.current_bead : ''})`).join(', ');
        const beadCounts = {
            open: (state.beads || []).filter(b => b.status === 'open' && b.project_id === projectId).length,
            in_progress: (state.beads || []).filter(b => b.status === 'in_progress' && b.project_id === projectId).length,
        };

        const systemPrompt = `You are the CEO dashboard assistant for Loom, an autonomous agent orchestration system.
Current project: ${project ? project.name : 'unknown'} (${projectId})
Agents: ${agentSummary || 'none'}
Beads: ${beadCounts.open} open, ${beadCounts.in_progress} in progress

To dispatch a task to an agent, the user should type: agent_name: task description
For example: "code reviewer: Review the authentication module for security issues"

Answer questions concisely about the system state, agents, and beads.`;

        const requestBody = {
            provider_id: providerId,
            messages: [
                { role: 'system', content: systemPrompt },
                { role: 'user', content: message }
            ]
        };

        await createStreamingRequest('/chat/completions', requestBody, {
            useStreaming: true,
            onChunk: (chunk, fullContent) => {
                responseEl.textContent = fullContent;
                responseEl.scrollTop = responseEl.scrollHeight;
            },
            onComplete: () => {
                responseEl.classList.remove('streaming');
                responseEl.classList.add('complete');
            },
            onError: (error) => {
                responseEl.classList.remove('streaming');
                responseEl.textContent += '\n\n[Error: ' + error + ']';
            }
        });
    } catch (e) {
        responseEl.classList.remove('streaming');
        responseEl.textContent = 'Request failed: ' + (e.message || 'Unknown error');
    } finally {
        setBusy('ceo-repl', false);
        sendBtn.disabled = false;
        sendBtn.textContent = 'Send';
    }
}

function renderPersonas() {
    // Filter out templates persona (it does nothing for now)
    const visiblePersonas = state.personas.filter(p => p.name !== 'templates');
    const html = visiblePersonas.map(persona => `
        <button type="button" class="persona-card" onclick="editPersona('${escapeHtml(persona.name)}')" aria-label="Edit persona: ${escapeHtml(persona.name)}">
            <h3>🎭 ${escapeHtml(persona.name)}</h3>
            <div>
                <strong>Autonomy:</strong> ${escapeHtml(persona.autonomy_level || 'N/A')}<br>
                <strong>Character:</strong> ${escapeHtml((persona.character || '').substring(0, 100))}...
            </div>
        </button>
    `).join('');
    
    document.getElementById('persona-list').innerHTML =
        html || renderEmptyState('No personas available', 'Add personas under ./personas to populate this list.');
}

async function cloneAgentPersona(agentId) {
    const agent = state.agents.find((a) => a.id === agentId);
    if (!agent) return;

    try {
        const res = await formModal({
            title: 'Clone agent persona',
            submitText: 'Clone',
            fields: [
                { id: 'new_persona_name', label: 'New persona name', type: 'text', required: true, placeholder: 'custom-qa-engineer' },
                { id: 'new_agent_name', label: 'New agent name (optional)', type: 'text', required: false, placeholder: `${agent.name}-custom` },
                { id: 'source_persona', label: 'Source persona (optional)', type: 'text', required: false, placeholder: 'default/qa-engineer' }
            ]
        });
        if (!res) return;

        const replace = await confirmModal({
            title: 'Replace current agent?',
            body: 'Replace this agent with the cloned persona? (Recommended to avoid duplicates.)',
            confirmText: 'Replace',
            cancelText: 'Keep both'
        });

        setBusy(`cloneAgent:${agentId}`, true);
        await apiCall(`/agents/${agentId}/clone`, {
            method: 'POST',
            body: JSON.stringify({
                new_persona_name: res.new_persona_name,
                new_agent_name: res.new_agent_name || '',
                source_persona: res.source_persona || '',
                replace: replace
            })
        });

        showToast('Persona cloned', 'success');
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`cloneAgent:${agentId}`, false);
    }
}

function renderDecisions() {
    const html = state.decisions.map(decision => {
        const p0Class = decision.priority === 0 ? 'p0' : '';
        const claimKey = `claimDecision:${decision.id}`;
        const decideKey = `makeDecision:${decision.id}`;
        return `
            <div class="decision-card ${p0Class}">
                <div class="decision-question">${escapeHtml(decision.question)}</div>
                <div>
                    <strong>Priority:</strong> P${decision.priority}<br>
                    <strong>Requester:</strong> ${decision.requester_id}<br>
                    ${decision.recommendation ? `<strong>Recommendation:</strong> ${escapeHtml(decision.recommendation)}` : ''}
                </div>
                <div class="decision-actions">
                    <button class="secondary" onclick="viewDecision('${decision.id}')">View</button>
                    <button onclick="claimDecision('${decision.id}')" ${isBusy(claimKey) ? 'disabled' : ''}>${isBusy(claimKey) ? 'Claiming…' : 'Claim'}</button>
                    ${decision.status === 'in_progress' ? `<button class="secondary" onclick="makeDecision('${decision.id}')" ${isBusy(decideKey) ? 'disabled' : ''}>${isBusy(decideKey) ? 'Submitting…' : 'Decide'}</button>` : ''}
                </div>
            </div>
        `;
    }).join('');

    document.getElementById('decision-list').innerHTML =
        html ||
        renderEmptyState('No pending decisions', 'Decision beads requiring input will appear here.');
}

function renderCeoDashboard() {
    const container = document.getElementById('ceo-dashboard-summary');
    if (!container) return;

    const providers = state.providers || [];
    const projects = state.projects || [];
    const beads = state.beads || [];
    const agents = state.agents || [];
    const decisions = state.decisions || [];

    const providerCounts = {
        active: providers.filter((p) => p.status === 'active').length,
        healthy: providers.filter((p) => p.status === 'healthy').length,
        failed: providers.filter((p) => p.status === 'failed').length
    };

    const beadCounts = {
        open: beads.filter((b) => b.status === 'open').length,
        in_progress: beads.filter((b) => b.status === 'in_progress').length,
        blocked: beads.filter((b) => b.status === 'blocked').length,
        closed: beads.filter((b) => b.status === 'closed').length
    };

    const agentCounts = {
        idle: agents.filter((a) => a.status === 'idle').length,
        working: agents.filter((a) => a.status === 'working').length,
        paused: agents.filter((a) => a.status === 'paused').length
    };

    const decisionCounts = {
        open: decisions.filter((d) => d.status === 'open').length,
        in_progress: decisions.filter((d) => d.status === 'in_progress').length,
        closed: decisions.filter((d) => d.status === 'closed').length
    };

    container.innerHTML = `
        <div class="ceo-dashboard-card">
            <h3>Providers</h3>
            <div class="small">Active: ${providerCounts.active} • Healthy: ${providerCounts.healthy} • Failed: ${providerCounts.failed}</div>
        </div>
        <div class="ceo-dashboard-card">
            <h3>Projects</h3>
            <div class="small">Total: ${projects.length}</div>
        </div>
        <div class="ceo-dashboard-card">
            <h3>Beads</h3>
            <div class="small">Open: ${beadCounts.open} • In progress: ${beadCounts.in_progress} • Blocked: ${beadCounts.blocked} • Closed: ${beadCounts.closed}</div>
        </div>
        <div class="ceo-dashboard-card">
            <h3>Agents</h3>
            <div class="small">Idle: ${agentCounts.idle} • Working: ${agentCounts.working} • Paused: ${agentCounts.paused}</div>
        </div>
        <div class="ceo-dashboard-card">
            <h3>Decisions</h3>
            <div class="small">Open: ${decisionCounts.open} • In progress: ${decisionCounts.in_progress} • Closed: ${decisionCounts.closed}</div>
        </div>
    `;
}

function viewDecision(decisionId) {
    const d = state.decisions.find((x) => x.id === decisionId);
    if (!d) return;

    const body = `
        <div>
            <div style="margin-bottom: 0.5rem;"><span class="badge priority-${d.priority}">P${d.priority}</span> <span class="badge">decision</span> <span class="badge">${escapeHtml(d.status || '')}</span></div>
            <div><strong>ID:</strong> ${escapeHtml(d.id)}</div>
            <div><strong>Requester:</strong> ${escapeHtml(d.requester_id || '')}</div>
            ${d.recommendation ? `<div style="margin-top: 0.5rem;"><strong>Recommendation:</strong> ${escapeHtml(d.recommendation)}</div>` : ''}
            ${Array.isArray(d.options) && d.options.length > 0 ? `<div style="margin-top: 0.5rem;"><strong>Options:</strong> ${d.options.map((o) => `<span class="badge">${escapeHtml(String(o))}</span>`).join(' ')}</div>` : ''}
            <div style="margin-top: 1rem; white-space: pre-wrap;">${escapeHtml(d.question || '')}</div>
        </div>
    `;

    openAppModal({
        title: 'Decision details',
        bodyHtml: body,
        actions: [
            { label: 'Close', variant: 'secondary', onClick: () => closeAppModal() },
            {
                label: 'Claim',
                onClick: async () => {
                    closeAppModal();
                    await claimDecision(decisionId);
                }
            }
        ]
    });
}

// Actions

async function showChangePasswordModal() {
    const values = await formModal({
        title: 'Change Password',
        submitText: 'Change',
        fields: [
            { id: 'old_password', label: 'Current Password', required: true, type: 'password', placeholder: 'Enter current password' },
            { id: 'new_password', label: 'New Password', required: true, type: 'password', placeholder: 'Enter new password (min 8 chars)' },
            { id: 'confirm_password', label: 'Confirm Password', required: true, type: 'password', placeholder: 'Re-enter new password' }
        ]
    });
    
    if (!values) return;
    
    const oldPwd = (values.old_password || '').trim();
    const newPwd = (values.new_password || '').trim();
    const confirmPwd = (values.confirm_password || '').trim();
    
    if (!oldPwd || !newPwd || !confirmPwd) {
        showToast('All fields are required', 'error');
        return;
    }
    
    if (newPwd.length < 8) {
        showToast('New password must be at least 8 characters', 'error');
        return;
    }
    
    if (newPwd !== confirmPwd) {
        showToast('Passwords do not match', 'error');
        return;
    }
    
    if (oldPwd === newPwd) {
        showToast('New password must be different from current password', 'error');
        return;
    }
    
    try {
        await apiCall('/auth/change-password', {
            method: 'POST',
            body: JSON.stringify({
                old_password: oldPwd,
                new_password: newPwd
            })
        });
        showToast('Password changed successfully', 'success');
    } catch (err) {
        showToast(`Failed to change password: ${err.message || 'Unknown error'}`, 'error');
    }
}

async function showRegisterProviderModal(preset = {}) {
    const values = await formModal({
        title: 'Register provider',
        submitText: 'Register',
        fields: [
            {
                id: 'endpoint',
                label: 'Provider URL',
                required: true,
                placeholder: preset.endpoint || 'e.g. http://myvllmhost.local:8000',
                description: 'OpenAI-compatible servers typically use /v1 (often :8000). Ollama defaults to :11434.'
            },
            { id: 'name', label: 'Display name (optional)', required: false, placeholder: preset.name || '' },
            { 
                id: 'type', 
                label: 'Protocol', 
                required: false, 
                type: 'select',
                options: [
                    { value: 'local', label: 'Local (vLLM)' },
                    { value: 'openai', label: 'OpenAI' },
                    { value: 'anthropic', label: 'Anthropic' },
                    { value: 'ollama', label: 'Ollama' },
                    { value: 'custom', label: 'Custom' }
                ],
                value: preset.type || 'local'
            },
            { id: 'api_key', label: 'API Key (optional)', required: false, type: 'password', placeholder: 'Leave blank if not required' },
            { id: 'model', label: 'Default model', required: false, placeholder: 'nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B-FP8', value: preset.model || 'nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B-FP8' }
        ]
    });
    
    if (!values) return;
    
    // Auto-generate provider ID from URL hostname
    const endpoint = (values.endpoint || '').trim();
    const providerType = (values.type || '').trim() || 'local';
    const endpointWarning = getProviderEndpointWarning({ endpoint, type: providerType });
    if (endpointWarning) {
        const ok = await confirmModal({
            title: 'Check provider endpoint',
            body: `${endpointWarning} Continue anyway?`,
            confirmText: 'Continue',
            cancelText: 'Edit'
        });
        if (!ok) return;
    }
    let providerId = endpoint;
    try {
        const url = new URL(endpoint);
        providerId = url.hostname.split('.')[0]; // e.g. "puck" from "http://puck.local:8000"
    } catch (e) {
        // Use endpoint as-is if not a valid URL
    }
    
    const payload = {
        id: providerId,
        name: (values.name || '').trim() || providerId,
        type: providerType,
        endpoint: endpoint,
        api_key: (values.api_key || '').trim() || '',
        model: (values.model || '').trim()
    };

    try {
        setBusy('registerProvider', true);
        await apiCall('/providers', { method: 'POST', body: JSON.stringify(payload) });
        showToast('Provider registered', 'success');
        await loadProviders();
        render();
    } catch (err) {
        showToast(`Failed to register provider: ${err.message}`, 'error');
    } finally {
        setBusy('registerProvider', false);
    }
}

async function fetchProviderModels(providerId) {
    try {
        setBusy(`providerModels:${providerId}`, true);
        const resp = await apiCall(`/providers/${providerId}/models`);
        const models = resp?.models || [];
        const body = models.length > 0
            ? `<div>${models.map((m) => `<div class="badge">${escapeHtml(m.id || '')}</div>`).join(' ')}</div>`
            : '<p>No models returned from provider.</p>';

        openAppModal({
            title: `Models: ${providerId}`,
            bodyHtml: body,
            actions: [{ label: 'Close', variant: 'secondary', onClick: () => closeAppModal() }]
        });
    } catch (err) {
        showToast(`Failed to fetch models: ${err.message || 'Unknown error'}`, 'error');
    } finally {
        setBusy(`providerModels:${providerId}`, false);
    }
}

async function renegotiateProvider(providerId) {
    try {
        setBusy(`providerNegotiate:${providerId}`, true);
        await apiCall(`/providers/${providerId}/negotiate`, { method: 'POST' });
        showToast('Provider negotiation complete', 'success');
        await loadProviders();
        render();
    } catch (err) {
        showToast(`Failed to negotiate provider: ${err.message || 'Unknown error'}`, 'error');
    } finally {
        setBusy(`providerNegotiate:${providerId}`, false);
    }
}

async function showEditProviderModal(providerId) {
    const provider = state.providers.find(p => p.id === providerId);
    if (!provider) return;

    try {
        const res = await formModal({
            title: `Edit Provider: ${provider.name || provider.id}`,
            submitText: 'Save Changes',
            fields: [
                { id: 'name', label: 'Display name', type: 'text', value: provider.name || '' },
                { id: 'endpoint', label: 'Provider URL', type: 'text', required: true, value: provider.endpoint || '' },
                {
                    id: 'type',
                    label: 'Protocol',
                    type: 'select',
                    value: provider.type || 'local',
                    options: [
                        { value: 'local', label: 'Local (vLLM)' },
                        { value: 'openai', label: 'OpenAI' },
                        { value: 'anthropic', label: 'Anthropic' },
                        { value: 'ollama', label: 'Ollama' },
                        { value: 'custom', label: 'Custom' }
                    ]
                },
                { id: 'model', label: 'Default model', type: 'text', value: provider.model || '' }
            ]
        });
        if (!res) return;

        await apiCall(`/providers/${providerId}`, {
            method: 'PUT',
            body: JSON.stringify({
                name: res.name,
                endpoint: res.endpoint,
                type: res.type,
                model: res.model
            })
        });

        showToast('Provider updated successfully', 'success');
        await loadProviders();
        render();
    } catch (e) {
        // handled
    }
}

async function deleteProvider(providerId) {
    const ok = await confirmModal({
        title: 'Delete provider?',
        body: `This will remove provider ${providerId}.`,
        confirmText: 'Delete',
        cancelText: 'Cancel',
        danger: true
    });
    if (!ok) return;

    try {
        setBusy(`deleteProvider:${providerId}`, true);
        await apiCall(`/providers/${providerId}`, { method: 'DELETE' });
        showToast('Provider deleted', 'success');
        await loadProviders();
        render();
    } catch (e) {
        // handled
    } finally {
        setBusy(`deleteProvider:${providerId}`, false);
    }
}

function showSpawnAgentModal() {
    // Populate persona and project dropdowns
    const personaSelect = document.getElementById('agent-persona');
    const projectSelect = document.getElementById('agent-project');
    const providerSelect = document.getElementById('agent-provider');
    
    personaSelect.innerHTML = state.personas.map(p => 
        `<option value="${escapeHtml(p.name)}">${escapeHtml(p.name)}</option>`
    ).join('');
    
    projectSelect.innerHTML = state.projects.map(p => 
        `<option value="${p.id}">${escapeHtml(p.name)}</option>`
    ).join('');

    providerSelect.innerHTML = state.providers.map(p =>
        `<option value="${escapeHtml(p.id)}">${escapeHtml(p.name || p.id)}</option>`
    ).join('');
    
    openModal('spawn-agent-modal', { initialFocusSelector: '#agent-name' });
}

function closeSpawnAgentModal() {
    closeModal('spawn-agent-modal');
}

document.getElementById('spawn-agent-form')?.addEventListener('submit', async (e) => {
    e.preventDefault();
    
    const formData = new FormData(e.target);
    const data = {
        name: formData.get('name'),
        persona_name: formData.get('persona_name'),
        project_id: formData.get('project_id'),
        provider_id: formData.get('provider_id')
    };
    
    try {
        setBusy('spawnAgent', true);

        const submitBtn = e.target.querySelector('button[type="submit"]');
        const prevText = submitBtn?.textContent;
        if (submitBtn) {
            submitBtn.disabled = true;
            submitBtn.textContent = 'Spawning…';
        }

        await apiCall('/agents', {
            method: 'POST',
            body: JSON.stringify(data)
        });

        showToast('Agent spawned', 'success');
        closeSpawnAgentModal();
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        const submitBtn = e.target.querySelector('button[type="submit"]');
        if (submitBtn) {
            submitBtn.disabled = false;
            submitBtn.textContent = submitBtn.textContent === 'Spawning…' ? 'Spawn Agent' : submitBtn.textContent;
        }
        setBusy('spawnAgent', false);
    }
});

async function stopAgent(agentId) {
    const ok = await confirmModal({
        title: 'Stop agent?',
        body: 'This will stop the agent and release its file locks.',
        confirmText: 'Stop agent',
        cancelText: 'Cancel',
        danger: true
    });
    if (!ok) return;
    
    try {
        setBusy(`stopAgent:${agentId}`, true);
        await apiCall(`/agents/${agentId}`, {
            method: 'DELETE'
        });

        showToast('Agent stopped', 'success');
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`stopAgent:${agentId}`, false);
    }
}

function viewBead(beadId) {
    const bead = state.beads.find(b => b.id === beadId);
    if (!bead) return;

    const statusClass = getCondensedBeadStatusClass(bead);
    const tagsValue = Array.isArray(bead.tags) ? bead.tags.join(', ') : '';
    const blockedByValue = Array.isArray(bead.blocked_by) ? bead.blocked_by.join(', ') : '';
    const blocksValue = Array.isArray(bead.blocks) ? bead.blocks.join(', ') : '';
    const relatedToValue = Array.isArray(bead.related_to) ? bead.related_to.join(', ') : '';
    const childrenValue = Array.isArray(bead.children) ? bead.children.join(', ') : '';
    const contextJson = bead.context ? JSON.stringify(bead.context, null, 2) : '';
    const dueDateValue = bead.due_date ? new Date(bead.due_date).toISOString().slice(0, 16) : '';

    const statusOptions = ['open', 'in_progress', 'blocked', 'closed', 'deferred', 'ready', 'tombstone', 'pinned']
        .map(s => `<option value="${s}"${bead.status === s ? ' selected' : ''}>${s}</option>`)
        .join('');
    const priorityOptions = [0, 1, 2, 3, 4]
        .map(p => `<option value="${p}"${bead.priority === p ? ' selected' : ''}>P${p}</option>`)
        .join('');

    const availableAgents = (state.agents || []).filter(a => a.status !== 'terminated');
    const agentOptions = '<option value="">-- select agent --</option>' +
        availableAgents.map(a => `<option value="${escapeHtml(a.id)}"${bead.assigned_to === a.id ? ' selected' : ''}>${escapeHtml(a.id)}</option>`).join('');

    const body = `
        <div class="bead-modal-viewer bead-condensed ${statusClass}">
            <div class="bead-modal-header">
                <code>${escapeHtml(bead.id)}</code>
                <span class="badge priority-${bead.priority}">P${bead.priority}</span>
                <span class="badge">${escapeHtml(bead.type)}</span>
                <span class="badge">${escapeHtml(bead.status)}</span>
            </div>

            <div class="bead-modal-fields">
                <label for="bead-modal-title">Title</label>
                <input id="bead-modal-title" type="text" value="${escapeHtml(bead.title || '')}" />

                <label for="bead-modal-type">Type</label>
                <input id="bead-modal-type" type="text" value="${escapeHtml(bead.type || '')}" />

                <label for="bead-modal-status">Status</label>
                <select id="bead-modal-status">${statusOptions}</select>

                <label for="bead-modal-priority">Priority</label>
                <select id="bead-modal-priority">${priorityOptions}</select>

                <label for="bead-modal-project">Project ID</label>
                <input id="bead-modal-project" type="text" value="${escapeHtml(bead.project_id || '')}" />

                <label for="bead-modal-parent">Parent</label>
                <input id="bead-modal-parent" type="text" value="${escapeHtml(bead.parent || '')}" placeholder="parent bead ID" />

                <label for="bead-modal-tags">Tags</label>
                <input id="bead-modal-tags" type="text" value="${escapeHtml(tagsValue)}" placeholder="comma-separated" />

                <label for="bead-modal-blocked-by">Blocked by</label>
                <input id="bead-modal-blocked-by" type="text" value="${escapeHtml(blockedByValue)}" placeholder="comma-separated bead IDs" />

                <label for="bead-modal-blocks">Blocks</label>
                <input id="bead-modal-blocks" type="text" value="${escapeHtml(blocksValue)}" placeholder="comma-separated bead IDs" />

                <label for="bead-modal-related">Related to</label>
                <input id="bead-modal-related" type="text" value="${escapeHtml(relatedToValue)}" placeholder="comma-separated bead IDs" />

                <label for="bead-modal-children">Children</label>
                <input id="bead-modal-children" type="text" value="${escapeHtml(childrenValue)}" placeholder="comma-separated bead IDs" />

                <label for="bead-modal-due">Due date</label>
                <input id="bead-modal-due" type="datetime-local" value="${dueDateValue}" />

                <label for="bead-modal-milestone">Milestone ID</label>
                <input id="bead-modal-milestone" type="text" value="${escapeHtml(bead.milestone_id || '')}" />

                <label for="bead-modal-estimate">Est. minutes</label>
                <input id="bead-modal-estimate" type="number" value="${bead.estimated_time || 0}" min="0" />

                <label for="bead-modal-desc">Description</label>
                <textarea id="bead-modal-desc" rows="4">${escapeHtml(bead.description || '')}</textarea>

                <label for="bead-modal-context">Context (JSON)</label>
                <textarea id="bead-modal-context" rows="3">${escapeHtml(contextJson)}</textarea>
            </div>

            <div class="bead-modal-assign">
                <strong>Agent Assignment</strong>
                <div class="bead-modal-assign-row">
                    <select id="bead-modal-agent">${agentOptions}</select>
                    <button type="button" id="bead-modal-dispatch-btn" class="secondary">Assign &amp; Dispatch</button>
                </div>
            </div>

            <details class="bead-modal-meta">
                <summary>Timestamps</summary>
                <div><strong>Created:</strong> ${bead.created_at ? new Date(bead.created_at).toLocaleString() : 'unknown'}</div>
                <div><strong>Updated:</strong> ${bead.updated_at ? new Date(bead.updated_at).toLocaleString() : 'unknown'}</div>
                <div><strong>Closed:</strong> ${bead.closed_at ? new Date(bead.closed_at).toLocaleString() : '<em>n/a</em>'}</div>
            </details>
        </div>
    `;

    openAppModal({
        title: 'Bead Details',
        bodyHtml: body,
        actions: [
            { label: 'Save Changes', variant: '', onClick: () => saveBeadFromModal(bead.id) },
            { label: 'Pair', variant: 'secondary', onClick: () => {
                const agentId = (document.getElementById('bead-modal-agent') || {}).value || '';
                closeAppModal();
                if (typeof openPairPanel === 'function') openPairPanel(bead.id, agentId);
            }},
            { label: 'Redispatch', variant: 'secondary', onClick: () => redispatchBead(bead.id) },
            { label: 'Close Bead', variant: 'secondary', onClick: () => closeBeadFromModal(bead.id) },
            { label: 'Dismiss', variant: 'secondary', onClick: () => closeAppModal() }
        ]
    });

    // Wire up dispatch button after modal is in the DOM
    const dispatchBtn = document.getElementById('bead-modal-dispatch-btn');
    if (dispatchBtn) {
        dispatchBtn.addEventListener('click', () => dispatchBeadFromModal(bead.id));
    }
}

async function saveBeadFromModal(beadId) {
    const val = (id) => (document.getElementById(id) || {}).value || '';
    const parseList = (v) => String(v).split(',').map(s => s.trim()).filter(s => s.length > 0);

    const contextRaw = val('bead-modal-context').trim();
    let parsedContext = {};
    if (contextRaw) {
        try {
            parsedContext = JSON.parse(contextRaw);
        } catch (e) {
            showToast('Context must be valid JSON.', 'error');
            return;
        }
    }

    const dueRaw = val('bead-modal-due');
    const payload = {
        title: val('bead-modal-title'),
        type: val('bead-modal-type'),
        status: val('bead-modal-status'),
        priority: Number(val('bead-modal-priority')),
        project_id: val('bead-modal-project'),
        parent: val('bead-modal-parent'),
        tags: parseList(val('bead-modal-tags')),
        blocked_by: parseList(val('bead-modal-blocked-by')),
        blocks: parseList(val('bead-modal-blocks')),
        related_to: parseList(val('bead-modal-related')),
        children: parseList(val('bead-modal-children')),
        milestone_id: val('bead-modal-milestone'),
        estimated_time: Number(val('bead-modal-estimate')) || 0,
        description: val('bead-modal-desc'),
        context: parsedContext
    };
    if (dueRaw) payload.due_date = new Date(dueRaw).toISOString();

    try {
        setBusy(`editBead:${beadId}`, true);
        await saveBeadUpdate(beadId, payload, { successMessage: 'Bead updated' });
        closeAppModal();
    } catch (error) {
        // Error already handled by saveBeadUpdate
    } finally {
        setBusy(`editBead:${beadId}`, false);
    }
}

async function dispatchBeadFromModal(beadId) {
    const agentSelect = document.getElementById('bead-modal-agent');
    const agentId = agentSelect ? agentSelect.value : '';
    if (!agentId) {
        showToast('Select an agent first.', 'error');
        return;
    }

    const ok = await confirmModal({
        title: 'Assign & Dispatch?',
        body: `Claim bead ${beadId} for agent ${agentId}? This sets status to in_progress and kicks the workflow engine.`,
        confirmText: 'Dispatch',
        cancelText: 'Cancel'
    });
    if (!ok) return;

    try {
        setBusy(`dispatchBead:${beadId}`, true);
        await apiCall(`/beads/${beadId}/claim`, {
            method: 'POST',
            body: JSON.stringify({ agent_id: agentId })
        });
        showToast('Bead dispatched to ' + agentId, 'success');
        closeAppModal();
        loadAll();
    } catch (error) {
        // Error already handled by apiCall
    } finally {
        setBusy(`dispatchBead:${beadId}`, false);
    }
}

async function closeBeadFromModal(beadId) {
    const ok = await confirmModal({
        title: 'Close bead?',
        body: `Close bead ${beadId}? This will mark it as closed.`,
        confirmText: 'Close',
        cancelText: 'Cancel',
        danger: true
    });
    if (!ok) return;

    try {
        setBusy(`closeBead:${beadId}`, true);
        await saveBeadUpdate(beadId, { status: 'closed' }, { successMessage: 'Bead closed' });
        closeAppModal();
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`closeBead:${beadId}`, false);
    }
}

async function dispatchBeadToAgent(beadId) {
    const select = document.getElementById(`ceo-dispatch-${beadId}`);
    const agentId = select ? select.value : '';
    if (!agentId) {
        showToast('Select an agent to dispatch.', 'error');
        return;
    }

    const ok = await confirmModal({
        title: 'Dispatch bead?',
        body: `Assign bead ${beadId} to ${agentId}?`,
        confirmText: 'Dispatch',
        cancelText: 'Cancel'
    });
    if (!ok) return;

    try {
        setBusy(`dispatchBead:${beadId}`, true);
        await saveBeadUpdate(beadId, { assigned_to: agentId, status: 'in_progress' }, { successMessage: 'Bead dispatched' });
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`dispatchBead:${beadId}`, false);
    }
}

async function closeBeadFromCeo(beadId) {
    const ok = await confirmModal({
        title: 'Close bead?',
        body: `Close bead ${beadId}? This will mark it as closed.`,
        confirmText: 'Close bead',
        cancelText: 'Cancel',
        danger: true
    });
    if (!ok) return;

    try {
        setBusy(`closeBead:${beadId}`, true);
        await saveBeadUpdate(beadId, { status: 'closed' }, { successMessage: 'Bead closed' });
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`closeBead:${beadId}`, false);
    }
}

async function showBeadCommentModal(beadId) {
    try {
        const res = await formModal({
            title: 'Add bead comment',
            submitText: 'Add comment',
            fields: [
                { id: 'comment', label: 'Comment', type: 'textarea', required: true, placeholder: 'Add context or instructions for this bead.' }
            ]
        });
        if (!res || !res.comment) return;

        setBusy(`commentBead:${beadId}`, true);
        const bead = await apiCall(`/beads/${beadId}`);
        const context = bead.context || {};
        const comments = parseBeadComments(context);

        comments.push({
            id: `comment-${Date.now()}`,
            author: 'ceo',
            timestamp: new Date().toISOString(),
            comment: String(res.comment || '').trim()
        });

        context.comments = JSON.stringify(comments);
        context.last_comment_by = 'ceo';
        context.last_comment_at = new Date().toISOString();

        await saveBeadUpdate(beadId, { context: context }, { successMessage: 'Comment added' });
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`commentBead:${beadId}`, false);
    }
}

function showAllBeadCommentsModal(beadId) {
    const bead = state.beads.find((b) => b.id === beadId);
    if (!bead) return;

    const comments = parseBeadComments(bead.context || {});
    const bodyHtml = comments.length
        ? `
            <div class="ceo-comment-list">
                ${comments
                    .map((c) => {
                        const author = escapeHtml(c.author || 'unknown');
                        const timestamp = escapeHtml((c.timestamp || '').substring(0, 19).replace('T', ' '));
                        const text = escapeHtml(c.comment || '');
                        return `
                            <div class="ceo-comment-item">
                                <div class="small"><strong>${author}</strong> · ${timestamp}</div>
                                <div class="ceo-comment-text">${text}</div>
                            </div>
                        `;
                    })
                    .join('')}
            </div>
        `
        : '<p class="small">No comments yet.</p>';

    openAppModal({
        title: `Comments for ${bead.title}`,
        bodyHtml: bodyHtml,
        actions: [
            { label: 'Close', variant: 'secondary', onClick: () => closeAppModal() }
        ]
    });
}

function openBeadDetails(beadId) {
    const bead = state.beads.find((b) => b.id === beadId);
    if (!bead) return;

    const bodyHtml = renderBeadDetailsHtml(bead);
    openAppModal({
        title: `Bead details: ${bead.title}`,
        bodyHtml,
        actions: [
            { label: 'Close', variant: 'secondary', onClick: () => closeAppModal() }
        ]
    });
}

function renderBeadDetailsHtml(bead) {
    const tags = Array.isArray(bead.tags) ? bead.tags : [];
    const blockedBy = Array.isArray(bead.blocked_by) ? bead.blocked_by : [];
    const blocks = Array.isArray(bead.blocks) ? bead.blocks : [];
    const relatedTo = Array.isArray(bead.related_to) ? bead.related_to : [];
    const children = Array.isArray(bead.children) ? bead.children : [];
    const context = bead.context ? JSON.stringify(bead.context, null, 2) : '';

    return `
        <div class="bead-details">
            <div class="bead-details-actions">
                <button type="button" class="icon-inline-button" onclick="openEditBeadModal('${escapeHtml(bead.id)}')" aria-label="Edit bead">✏️</button>
            </div>
            <div><strong>ID:</strong> ${escapeHtml(bead.id || '')}</div>
            <div><strong>Title:</strong> ${escapeHtml(bead.title || '')}</div>
            <div><strong>Type:</strong> ${escapeHtml(bead.type || '')}</div>
            <div><strong>Status:</strong> ${escapeHtml(bead.status || '')}</div>
            <div><strong>Priority:</strong> ${escapeHtml(String(bead.priority ?? ''))}</div>
            <div><strong>Project:</strong> ${escapeHtml(bead.project_id || '')}</div>
            <div><strong>Assigned to:</strong> ${escapeHtml(bead.assigned_to || '')}</div>
            <div><strong>Parent:</strong> ${escapeHtml(bead.parent || '')}</div>
            <div><strong>Tags:</strong> ${escapeHtml(tags.join(', '))}</div>
            <div><strong>Blocked by:</strong> ${escapeHtml(blockedBy.join(', '))}</div>
            <div><strong>Blocks:</strong> ${escapeHtml(blocks.join(', '))}</div>
            <div><strong>Related to:</strong> ${escapeHtml(relatedTo.join(', '))}</div>
            <div><strong>Children:</strong> ${escapeHtml(children.join(', '))}</div>
            <div><strong>Created at:</strong> ${escapeHtml(String(bead.created_at || ''))}</div>
            <div><strong>Updated at:</strong> ${escapeHtml(String(bead.updated_at || ''))}</div>
            <div><strong>Closed at:</strong> ${escapeHtml(String(bead.closed_at || ''))}</div>
            <div style="margin-top: 0.5rem;"><strong>Description:</strong></div>
            <div style="white-space: pre-wrap;">${escapeHtml(bead.description || '')}</div>
            <div style="margin-top: 0.5rem;"><strong>Context:</strong></div>
            <pre class="small" style="white-space: pre-wrap;">${escapeHtml(context)}</pre>
        </div>
    `;
}

function getCondensedBeadStatusClass(bead) {
    const status = String(bead.status || '').toLowerCase();
    const blocks = Array.isArray(bead.blocks) ? bead.blocks : [];
    const classes = [];
    if (status) classes.push(`bead-status-${status}`);
    if (blocks.length > 0) classes.push('bead-status-blocks');
    return classes.join(' ');
}

async function openEditBeadModal(beadId) {
    const bead = state.beads.find((b) => b.id === beadId);
    if (!bead) return;

    const tagsValue = Array.isArray(bead.tags) ? bead.tags.join(', ') : '';
    const blockedByValue = Array.isArray(bead.blocked_by) ? bead.blocked_by.join(', ') : '';
    const blocksValue = Array.isArray(bead.blocks) ? bead.blocks.join(', ') : '';
    const relatedToValue = Array.isArray(bead.related_to) ? bead.related_to.join(', ') : '';
    const childrenValue = Array.isArray(bead.children) ? bead.children.join(', ') : '';
    const contextValue = bead.context ? JSON.stringify(bead.context, null, 2) : '';

    const res = await formModal({
        title: 'Edit bead',
        submitText: 'Save',
        cancelText: 'Cancel',
        fields: [
            { id: 'title', label: 'Title', type: 'text', required: true, value: bead.title || '' },
            { id: 'type', label: 'Type', type: 'text', required: true, value: bead.type || '' },
            { id: 'status', label: 'Status', type: 'select', required: true, value: bead.status || 'open', options: [
                { value: 'open', label: 'open' },
                { value: 'in_progress', label: 'in_progress' },
                { value: 'blocked', label: 'blocked' },
                { value: 'closed', label: 'closed' }
            ]},
            { id: 'priority', label: 'Priority', type: 'number', required: true, value: bead.priority ?? 2 },
            { id: 'project_id', label: 'Project ID', type: 'text', required: true, value: bead.project_id || '' },
            { id: 'assigned_to', label: 'Assigned to', type: 'text', required: false, value: bead.assigned_to || '' },
            { id: 'parent', label: 'Parent', type: 'text', required: false, value: bead.parent || '' },
            { id: 'tags', label: 'Tags', type: 'text', required: false, value: tagsValue, description: 'Comma-separated' },
            { id: 'blocked_by', label: 'Blocked by', type: 'text', required: false, value: blockedByValue, description: 'Comma-separated bead IDs' },
            { id: 'blocks', label: 'Blocks', type: 'text', required: false, value: blocksValue, description: 'Comma-separated bead IDs' },
            { id: 'related_to', label: 'Related to', type: 'text', required: false, value: relatedToValue, description: 'Comma-separated bead IDs' },
            { id: 'children', label: 'Children', type: 'text', required: false, value: childrenValue, description: 'Comma-separated bead IDs' },
            { id: 'description', label: 'Description', type: 'textarea', required: false, value: bead.description || '' },
            { id: 'context', label: 'Context (JSON)', type: 'textarea', required: false, value: contextValue }
        ]
    });
    if (!res) return;

    let parsedContext = {};
    if ((res.context || '').trim()) {
        try {
            parsedContext = JSON.parse(res.context);
        } catch (e) {
            showToast('Context must be valid JSON.', 'error');
            return;
        }
    }

    const parseList = (value) => {
        return String(value || '')
            .split(',')
            .map((v) => v.trim())
            .filter((v) => v.length > 0);
    };

    const payload = {
        title: res.title,
        type: res.type,
        status: res.status,
        priority: Number(res.priority),
        project_id: res.project_id,
        assigned_to: res.assigned_to || '',
        parent: res.parent || '',
        tags: parseList(res.tags),
        blocked_by: parseList(res.blocked_by),
        blocks: parseList(res.blocks),
        related_to: parseList(res.related_to),
        children: parseList(res.children),
        description: res.description || '',
        context: parsedContext
    };

    try {
        setBusy(`editBead:${beadId}`, true);
        await saveBeadUpdate(beadId, payload, { successMessage: 'Bead updated' });
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`editBead:${beadId}`, false);
    }
}

async function redispatchBead(beadId) {
    try {
        const res = await formModal({
            title: 'Request redispatch',
            submitText: 'Request',
            fields: [{ id: 'reason', label: 'Reason (optional)', type: 'textarea', required: false, placeholder: 'Why should this bead be rerun?' }]
        });
        if (!res) return;

        setBusy(`redispatchBead:${beadId}`, true);
        await apiCall(`/beads/${beadId}/redispatch`, {
            method: 'POST',
            body: JSON.stringify({ reason: res.reason || '' })
        });

        showToast('Redispatch requested', 'success');
        closeAppModal();
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`redispatchBead:${beadId}`, false);
    }
}

async function escalateBead(beadId) {
    try {
        const res = await formModal({
            title: 'Escalate to CEO',
            submitText: 'Escalate',
            fields: [
                { id: 'reason', label: 'Decision needed / reason', type: 'textarea', required: true, placeholder: 'What decision is required?' },
                { id: 'returned_to', label: 'Return to (agent/user id, optional)', type: 'text', required: false, placeholder: 'agent-123 or user-jordan' }
            ]
        });
        if (!res) return;

        setBusy(`escalateBead:${beadId}`, true);
        await apiCall(`/beads/${beadId}/escalate`, {
            method: 'POST',
            body: JSON.stringify({ reason: res.reason, returned_to: res.returned_to || '' })
        });

        showToast('Escalated to CEO (decision created)', 'success');
        closeAppModal();
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`escalateBead:${beadId}`, false);
    }
}

async function claimDecision(decisionId) {
    try {
        const res = await formModal({
            title: 'Claim decision',
            submitText: 'Claim',
            fields: [
                {
                    id: 'agent_id',
                    label: 'Your user ID (or agent ID)',
                    type: 'text',
                    required: true,
                    placeholder: 'user-jordan or agent-123'
                }
            ]
        });
        if (!res) return;

        setBusy(`claimDecision:${decisionId}`, true);
        await apiCall(`/beads/${decisionId}/claim`, {
            method: 'POST',
            body: JSON.stringify({ agent_id: res.agent_id })
        });

        showToast('Decision claimed', 'success');
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`claimDecision:${decisionId}`, false);
    }
}

async function makeDecision(decisionId) {
    try {
        const res = await formModal({
            title: 'Make decision',
            submitText: 'Submit decision',
            fields: [
                { id: 'decision', label: 'Decision', type: 'text', required: true, placeholder: 'APPROVE / DENY / ...' },
                { id: 'rationale', label: 'Rationale', type: 'textarea', required: true, placeholder: 'Why?' },
                { id: 'decider_id', label: 'Your user ID', type: 'text', required: true, placeholder: 'user-jordan' }
            ]
        });
        if (!res) return;

        setBusy(`makeDecision:${decisionId}`, true);
        await apiCall(`/decisions/${decisionId}/decide`, {
            method: 'POST',
            body: JSON.stringify({
                decider_id: res.decider_id,
                decision: res.decision,
                rationale: res.rationale
            })
        });

        showToast('Decision submitted', 'success');
        loadAll();
    } catch (error) {
        // Error already handled
    } finally {
        setBusy(`makeDecision:${decisionId}`, false);
    }
}

async function editPersona(personaName) {
    const persona = state.personas.find(p => p.name === personaName);
    if (!persona) return;

    try {
        const res = await formModal({
            title: `Edit Persona: ${persona.name}`,
            submitText: 'Save Changes',
            fields: [
                { id: 'character', label: 'Character', type: 'textarea', rows: 6, required: true, value: persona.character || '' },
                { id: 'autonomy_level', label: 'Autonomy Level', type: 'select', required: true, value: persona.autonomy_level || 'moderate',
                  options: [
                    { value: 'minimal', label: 'Minimal - Needs constant guidance' },
                    { value: 'moderate', label: 'Moderate - Some independence' },
                    { value: 'high', label: 'High - Highly autonomous' },
                    { value: 'full', label: 'Full - Completely autonomous' }
                  ]
                },
                { id: 'instructions', label: 'Instructions', type: 'textarea', rows: 10, value: persona.instructions || '' },
                { id: 'tools', label: 'Available Tools (comma-separated)', type: 'text', value: (persona.tools || []).join(', ') }
            ]
        });
        if (!res) return;

        const tools = res.tools ? res.tools.split(',').map(t => t.trim()).filter(t => t) : [];

        await apiCall(`/personas/${personaName}`, {
            method: 'PUT',
            body: JSON.stringify({
                character: res.character,
                autonomy_level: res.autonomy_level,
                instructions: res.instructions,
                tools: tools
            })
        });

        showToast('Persona updated successfully', 'success');
        await loadPersonas();
        render();
    } catch (e) {
        // handled
    }
}

function closePersonaModal() {
    closeModal('persona-modal');
}

// Utilities
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Close modals when clicking outside
window.onclick = function(event) {
    const spawnModal = document.getElementById('spawn-agent-modal');
    const personaModal = document.getElementById('persona-modal');
    const appModal = document.getElementById('app-modal');
    
    if (event.target === spawnModal) {
        closeSpawnAgentModal();
    }
    if (event.target === personaModal) {
        closePersonaModal();
    }
    if (event.target === appModal) {
        closeAppModal();
    }
}

function getFocusableElements(root) {
    const selector = [
        'a[href]',
        'button:not([disabled])',
        'input:not([disabled])',
        'select:not([disabled])',
        'textarea:not([disabled])',
        '[tabindex]:not([tabindex="-1"])'
    ].join(',');

    return Array.from(root.querySelectorAll(selector)).filter(el => {
        // Skip elements that are hidden via display:none.
        return el.offsetParent !== null || el === document.activeElement;
    });
}

function openModal(modalId, options = {}) {
    const modal = document.getElementById(modalId);
    if (!modal) return;

    modalState.lastFocused = document.activeElement;
    modalState.activeId = modalId;

    modal.style.display = 'block';
    modal.classList.add('show');
    modal.setAttribute('aria-hidden', 'false');
    document.body.style.overflow = 'hidden';

    const initial = options.initialFocusSelector ? modal.querySelector(options.initialFocusSelector) : null;
    (initial || modal).focus();
}

function closeModal(modalId) {
    const modal = document.getElementById(modalId);
    if (!modal) return;
    modal.style.display = 'none';
    modal.classList.remove('show');
    modal.setAttribute('aria-hidden', 'true');
    document.body.style.overflow = '';

    modalState.activeId = null;

    if (modalState.lastFocused && typeof modalState.lastFocused.focus === 'function') {
        modalState.lastFocused.focus();
    }
    modalState.lastFocused = null;
}

document.addEventListener('keydown', (event) => {
    if (!modalState.activeId) return;

    const modal = document.getElementById(modalState.activeId);
    if (!modal) return;

    if (event.key === 'Escape') {
        event.preventDefault();
        closeModal(modalState.activeId);
        return;
    }

    if (event.key !== 'Tab') return;

    const focusables = getFocusableElements(modal);
    if (focusables.length === 0) {
        event.preventDefault();
        return;
    }

    const first = focusables[0];
    const last = focusables[focusables.length - 1];

    if (event.shiftKey) {
        if (document.activeElement === first || document.activeElement === modal) {
            event.preventDefault();
            last.focus();
        }
    } else {
        if (document.activeElement === last) {
            event.preventDefault();
            first.focus();
        }
    }
});

function closeAppModal() {
    if (typeof stopConversationAutoRefresh === 'function') stopConversationAutoRefresh();
    // Reset modal width if it was changed
    const modalContent = document.querySelector('#app-modal .modal-content');
    if (modalContent) modalContent.style.maxWidth = '';
    closeModal('app-modal');
}

function openAppModal({ title, bodyHtml, actions = [] }) {
    const titleEl = document.getElementById('app-modal-title');
    const bodyEl = document.getElementById('app-modal-body');
    const actionsEl = document.getElementById('app-modal-actions');
    if (!titleEl || !bodyEl || !actionsEl) return;

    titleEl.textContent = title || 'Dialog';
    bodyEl.innerHTML = bodyHtml || '';
    actionsEl.innerHTML = '';

    for (const a of actions) {
        const btn = document.createElement('button');
        btn.type = 'button';
        if (a.variant) btn.className = a.variant;
        btn.textContent = a.label;
        btn.addEventListener('click', a.onClick);
        actionsEl.appendChild(btn);
    }

    openModal('app-modal');
}

function confirmModal({ title, body, confirmText = 'Confirm', cancelText = 'Cancel', danger = false }) {
    return new Promise((resolve) => {
        openAppModal({
            title,
            bodyHtml: `<p>${escapeHtml(body || '')}</p>`,
            actions: [
                {
                    label: cancelText,
                    variant: 'secondary',
                    onClick: () => {
                        closeAppModal();
                        resolve(false);
                    }
                },
                {
                    label: confirmText,
                    variant: danger ? 'danger' : '',
                    onClick: () => {
                        closeAppModal();
                        resolve(true);
                    }
                }
            ]
        });
    });
}

function formModal({ title, submitText = 'Submit', cancelText = 'Cancel', fields = [] }) {
    return new Promise((resolve) => {
        const formId = `modal-form-${Math.random().toString(16).slice(2)}`;
        const bodyHtml = `
            <form id="${formId}">
                ${fields
                    .map((f) => {
                        const id = `field-${formId}-${f.id}`;
                        const required = f.required ? 'required' : '';
                        const placeholder = f.placeholder ? `placeholder="${escapeHtml(f.placeholder)}"` : '';
                        const readOnly = f.readonly ? 'readonly' : '';
                        const disabled = f.disabled ? 'disabled' : '';
                        const value = f.value !== undefined && f.value !== null ? String(f.value) : '';
                        const description = f.description ? `<div class="small" style="color: var(--text-muted); margin-top: 0.25rem;">${escapeHtml(f.description)}</div>` : '';
                        
                        if (f.type === 'textarea') {
                            return `
                                <label for="${id}">${escapeHtml(f.label)}</label>
                                <textarea id="${id}" name="${escapeHtml(f.id)}" ${required} ${placeholder} ${readOnly} ${disabled}>${escapeHtml(value)}</textarea>
                                ${description}
                            `;
                        }
                        if (f.type === 'select') {
                            const options = Array.isArray(f.options) ? f.options : [];
                            return `
                                <label for="${id}">${escapeHtml(f.label)}</label>
                                <select id="${id}" name="${escapeHtml(f.id)}" ${required} ${disabled}>
                                    ${options
                                        .map((opt) => {
                                            const optValue = String(opt.value ?? '');
                                            const selected = optValue === value ? 'selected' : '';
                                            return `<option value="${escapeHtml(optValue)}" ${selected}>${escapeHtml(opt.label ?? optValue)}</option>`;
                                        })
                                        .join('')}
                                </select>
                                ${description}
                            `;
                        }
                        if (f.type === 'checkbox') {
                            const checked = (value === 'true' || value === true || value === '1') ? 'checked' : '';
                            return `
                                <div style="display: flex; align-items: center; gap: 0.5rem;">
                                    <input type="checkbox" id="${id}" name="${escapeHtml(f.id)}" value="true" ${checked} ${disabled}>
                                    <label for="${id}" style="margin: 0;">${escapeHtml(f.label)}</label>
                                </div>
                                ${description}
                            `;
                        }
                        // Default: text, password, number, etc.
                        const inputType = f.type || 'text';
                        return `
                            <label for="${id}">${escapeHtml(f.label)}</label>
                            <input type="${escapeHtml(inputType)}" id="${id}" name="${escapeHtml(f.id)}" ${required} ${placeholder} ${readOnly} ${disabled} value="${escapeHtml(value)}">
                            ${description}
                        `;
                    })
                    .join('')}
            </form>
        `;

        openAppModal({
            title,
            bodyHtml,
            actions: [
                {
                    label: cancelText,
                    variant: 'secondary',
                    onClick: () => {
                        closeAppModal();
                        resolve(null);
                    }
                },
                {
                    label: submitText,
                    onClick: () => {
                        const form = document.getElementById(formId);
                        if (!form) return;
                        if (!form.reportValidity()) return;
                        const data = new FormData(form);
                        const out = {};
                        for (const [k, v] of data.entries()) out[k] = String(v);
                        closeAppModal();
                        resolve(out);
                    }
                }
            ]
        });

        // focus first field
        window.setTimeout(() => {
            const form = document.getElementById(formId);
            const first = form?.querySelector('input, textarea, select');
            if (first) first.focus();
        }, 0);
    });
}

// ============================================================================
// User Management Functions
// ============================================================================

function renderUsers() {
    const container = document.getElementById('users-list');
    if (!container) return;

    const users = state.users || [];
    const apiKeys = state.apiKeys || [];

    if (users.length === 0) {
        container.innerHTML = renderEmptyState(
            'No Users',
            'User management requires admin access. Login as admin to view users.',
            ''
        );
        return;
    }

    container.innerHTML = `
        <table class="data-table" style="margin-top: 1rem;">
            <thead>
                <tr>
                    <th>Username</th>
                    <th>Email</th>
                    <th>Role</th>
                    <th>Status</th>
                    <th>Created</th>
                    <th>Updated</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
                ${users.map(user => `
                    <tr>
                        <td><strong>${escapeHtml(user.username)}</strong></td>
                        <td>${escapeHtml(user.email || '-')}</td>
                        <td><span class="badge badge-${getRoleBadgeClass(user.role)}">${escapeHtml(user.role)}</span></td>
                        <td>${user.is_active ? '<span style="color: var(--success-color);">Active</span>' : '<span style="color: var(--text-muted);">Inactive</span>'}</td>
                        <td class="small">${formatDate(user.created_at)}</td>
                        <td class="small">${formatDate(user.updated_at)}</td>
                        <td>
                            <div class="action-buttons">
                                <button type="button" class="btn-icon" onclick="showEditUserModal('${escapeHtml(user.id || user.username)}')" title="Edit user">✏️</button>
                                <button type="button" class="btn-icon btn-danger" onclick="confirmDeleteUser('${escapeHtml(user.id || user.username)}')" title="Delete user">🗑️</button>
                            </div>
                        </td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
    `;

    // Render API keys
    const apikeysContainer = document.getElementById('apikeys-list');
    if (!apikeysContainer) return;

    if (apiKeys.length === 0) {
        apikeysContainer.innerHTML = `<p class="small" style="color: var(--text-muted); margin-top: 1rem;">No API keys. Generate one to access the API programmatically.</p>`;
        return;
    }

    apikeysContainer.innerHTML = `
        <table class="data-table" style="margin-top: 1rem;">
            <thead>
                <tr>
                    <th>Name</th>
                    <th>Key Prefix</th>
                    <th>Permissions</th>
                    <th>Status</th>
                    <th>Expires</th>
                    <th>Last Used</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
                ${apiKeys.map(key => {
                    const isExpired = key.expires_at && new Date(key.expires_at) < new Date();
                    return `
                        <tr>
                            <td><strong>${escapeHtml(key.name)}</strong></td>
                            <td><code>${escapeHtml(key.key_prefix)}...</code></td>
                            <td class="small">${(key.permissions || []).join(', ')}</td>
                            <td>${key.is_active && !isExpired ? '<span style="color: var(--success-color);">Active</span>' : '<span style="color: var(--error-color);">Inactive</span>'}</td>
                            <td class="small">${key.expires_at ? formatDate(key.expires_at) : 'Never'}</td>
                            <td class="small">${key.last_used ? formatDate(key.last_used) : 'Never'}</td>
                            <td><button type="button" class="secondary small" onclick="revokeAPIKey('${escapeHtml(key.id)}')">Revoke</button></td>
                        </tr>
                    `;
                }).join('')}
            </tbody>
        </table>
    `;
}

function getRoleBadgeClass(role) {
    const map = {
        admin: 'error',
        user: 'info',
        viewer: 'warning',
        service: 'neutral'
    };
    return map[role] || 'neutral';
}

function renderDiagrams() {
    // Update project filter dropdown
    const projectFilter = document.getElementById('diagram-project-filter');
    if (projectFilter && state.projects) {
        const currentValue = projectFilter.value;
        projectFilter.innerHTML = '<option value="all">All Projects</option>' +
            state.projects.map(p =>
                `<option value="${escapeHtml(p.id)}">${escapeHtml(p.name || p.id)}</option>`
            ).join('');
        // Restore previous selection if it still exists
        if (currentValue && Array.from(projectFilter.options).some(opt => opt.value === currentValue)) {
            projectFilter.value = currentValue;
        }
    }

    // Update legend visibility based on current diagram type
    if (typeof diagramState !== 'undefined') {
        const legendHierarchy = document.getElementById('legend-hierarchy');
        const legendMotivation = document.getElementById('legend-motivation');
        const legendMessage = document.getElementById('legend-message');

        if (legendHierarchy) legendHierarchy.style.display = diagramState.currentType === 'hierarchy' ? 'flex' : 'none';
        if (legendMotivation) legendMotivation.style.display = diagramState.currentType === 'motivation' ? 'flex' : 'none';
        if (legendMessage) legendMessage.style.display = diagramState.currentType === 'message' ? 'flex' : 'none';
    }

    // If diagrams view is active, update diagram data
    if (uiState.view.active === 'diagrams' && typeof updateDiagramData === 'function') {
        updateDiagramData();
    }
}

async function handleCreateUser() {
    const username = document.getElementById('user-username')?.value;
    const email = document.getElementById('user-email')?.value;
    const password = document.getElementById('user-password')?.value;
    const role = document.getElementById('user-role')?.value;

    if (!username || !password || !role) {
        showToast('Username, password, and role are required', 'error');
        return;
    }

    try {
        await apiCall('/auth/users', {
            method: 'POST',
            body: JSON.stringify({
                username,
                email,
                password,
                role
            })
        });

        showToast(`User ${username} created successfully`, 'success');
        document.getElementById('create-user-form').style.display = 'none';
        document.getElementById('user-form').reset();
        await loadUsers();
        render();
    } catch (e) {
        showToast(`Failed to create user: ${e.message}`, 'error');
    }
}

async function handleCreateAPIKey() {
    const name = document.getElementById('apikey-name')?.value;
    const permissionsEl = document.getElementById('apikey-permissions');
    const expiresIn = parseInt(document.getElementById('apikey-expires')?.value || '0');

    if (!name) {
        showToast('API key name is required', 'error');
        return;
    }

    const permissions = Array.from(permissionsEl?.selectedOptions || []).map(opt => opt.value);

    try {
        const response = await apiCall('/auth/api-keys', {
            method: 'POST',
            body: JSON.stringify({
                name,
                permissions,
                expires_in: expiresIn
            })
        });

        // Hide form, show key display
        document.getElementById('create-apikey-form').style.display = 'none';
        document.getElementById('apikey-form').reset();
        
        const display = document.getElementById('apikey-display');
        const valueEl = document.getElementById('apikey-value');
        if (display && valueEl && response.key) {
            valueEl.textContent = response.key;
            display.style.display = 'block';
        }

        showToast('API key created successfully', 'success');
        await loadAPIKeys();
        render();
    } catch (e) {
        showToast(`Failed to create API key: ${e.message}`, 'error');
    }
}

async function revokeAPIKey(keyId) {
    if (!confirm('Are you sure you want to revoke this API key? This action cannot be undone.')) {
        return;
    }

    try {
        await apiCall(`/auth/api-keys/${keyId}`, {
            method: 'DELETE'
        });

        showToast('API key revoked', 'success');
        await loadAPIKeys();
        render();
    } catch (e) {
        showToast(`Failed to revoke API key: ${e.message}`, 'error');
    }
}

// Analytics Dashboard Functions
async function loadAnalytics() {
    const timeRange = document.getElementById('analytics-time-range').value;
    let startTime, endTime;
    
    const now = new Date();
    endTime = now.toISOString();
    
    switch (timeRange) {
        case '1h':
            startTime = new Date(now - 60 * 60 * 1000).toISOString();
            break;
        case '24h':
            startTime = new Date(now - 24 * 60 * 60 * 1000).toISOString();
            break;
        case '7d':
            startTime = new Date(now - 7 * 24 * 60 * 60 * 1000).toISOString();
            break;
        case '30d':
            startTime = new Date(now - 30 * 24 * 60 * 60 * 1000).toISOString();
            break;
        case 'custom':
            startTime = document.getElementById('analytics-start-time').value;
            endTime = document.getElementById('analytics-end-time').value;
            if (!startTime || !endTime) {
                showToast('Please select start and end times', 'warning');
                return;
            }
            break;
    }
    
    try {
        const params = new URLSearchParams();
        if (startTime) params.append('start_time', startTime);
        if (endTime) params.append('end_time', endTime);
        
        const response = await fetch(`/api/v1/analytics/stats?${params}`, {
            headers: {
                'Authorization': `Bearer ${state.token}`
            }
        });
        
        if (!response.ok) throw new Error('Failed to load analytics');
        
        const stats = await response.json();
        renderAnalytics(stats);
        
        const batchingParams = new URLSearchParams(params.toString());
        batchingParams.append('auto_batch', 'true');
        await loadBatchingRecommendations(batchingParams);
    } catch (error) {
        console.error('Error loading analytics:', error);
        showToast('Failed to load analytics: ' + error.message, 'error');
    }
}

function renderAnalytics(stats) {
    // Update summary cards
    document.getElementById('analytics-total-requests').textContent = 
        stats.total_requests.toLocaleString();
    
    document.getElementById('analytics-total-cost').textContent = 
        '$' + stats.total_cost_usd.toFixed(2);
    
    document.getElementById('analytics-avg-latency').textContent = 
        Math.round(stats.avg_latency_ms);
    
    document.getElementById('analytics-error-rate').textContent = 
        (stats.error_rate * 100).toFixed(1);
    
    // Render charts
    renderBarChart('chart-cost-by-provider', stats.cost_by_provider || {}, '$');
    renderBarChart('chart-cost-by-user', stats.cost_by_user || {}, '$');
    renderBarChart('chart-requests-by-provider', stats.requests_by_provider || {}, '');
    renderBarChart('chart-requests-by-user', stats.requests_by_user || {}, '');
    
    // Render detailed table
    renderAnalyticsTable(stats);
}

async function loadBatchingRecommendations(params) {
    const tbody = document.getElementById('analytics-batching-table');
    try {
        const query = params ? params.toString() : '';
        const response = await fetch(`/api/v1/analytics/batching?${query}`, {
            headers: {
                'Authorization': `Bearer ${state.token}`
            }
        });
        
        if (!response.ok) throw new Error('Failed to load batching recommendations');
        
        const recommendations = await response.json();
        renderBatchingRecommendations(recommendations);
    } catch (error) {
        console.error('Error loading batching recommendations:', error);
        if (tbody) {
            tbody.innerHTML = '<tr><td colspan="6" style="text-align: center;">Unable to load batching recommendations</td></tr>';
        }
        showToast('Failed to load batching recommendations: ' + error.message, 'error');
    }
}

function renderBatchingRecommendations(data) {
    const summary = data?.summary || {};
    const requests = summary.batchable_requests || 0;
    const batches = summary.recommended_batches || 0;
    const savings = summary.estimated_cost_savings_usd || 0;
    const avgBatchSize = summary.average_batch_size || 0;

    const requestsEl = document.getElementById('analytics-batching-requests');
    const batchesEl = document.getElementById('analytics-batching-batches');
    const savingsEl = document.getElementById('analytics-batching-savings');
    const avgBatchEl = document.getElementById('analytics-batching-batch-size');

    if (requestsEl) requestsEl.textContent = requests.toLocaleString();
    if (batchesEl) batchesEl.textContent = batches.toLocaleString();
    if (savingsEl) savingsEl.textContent = '$' + savings.toFixed(4);
    if (avgBatchEl) avgBatchEl.textContent = avgBatchSize ? avgBatchSize.toFixed(1) : '-';

    const tbody = document.getElementById('analytics-batching-table');
    if (!tbody) return;

    const recommendations = data?.recommendations || [];
    if (recommendations.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" style="text-align: center;">No batchable requests detected</td></tr>';
        return;
    }

    tbody.innerHTML = recommendations.slice(0, 10).map((rec) => {
        const endpoint = `${rec.method || ''} ${rec.path || ''}`.trim();
        const provider = rec.provider_id || 'unknown';
        const window = formatBatchingWindow(rec.time_window_start, rec.time_window_end);
        const savingsValue = rec.estimated_cost_savings_usd || 0;
        return `
            <tr>
                <td>${escapeHtml(endpoint || '-')}
                    <div class="small" style="color: var(--text-muted);">${escapeHtml(rec.model_name || 'model n/a')}</div>
                </td>
                <td>${escapeHtml(provider)}</td>
                <td>${(rec.request_count || 0).toLocaleString()}</td>
                <td>${rec.batch_size || '-'}</td>
                <td>$${savingsValue.toFixed(4)}</td>
                <td class="small">${escapeHtml(window)}</td>
            </tr>
        `;
    }).join('');
}

function formatBatchingWindow(start, end) {
    if (!start || !end) return '-';
    const startDate = new Date(start);
    const endDate = new Date(end);
    if (Number.isNaN(startDate.getTime()) || Number.isNaN(endDate.getTime())) {
        return '-';
    }
    const startTime = startDate.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    const endTime = endDate.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    return `${startTime} - ${endTime}`;
}

function renderBarChart(elementId, data, prefix = '') {
    const container = document.getElementById(elementId);
    if (!container) return;
    
    const entries = Object.entries(data).sort((a, b) => b[1] - a[1]);
    
    if (entries.length === 0) {
        container.innerHTML = '<p class="small" style="text-align: center; padding: 2rem;">No data available</p>';
        return;
    }
    
    const maxValue = Math.max(...entries.map(([, value]) => value));
    
    container.innerHTML = entries.map(([key, value]) => {
        const percentage = maxValue > 0 ? (value / maxValue) * 100 : 0;
        const displayValue = prefix ? `${prefix}${value.toFixed(2)}` : value;
        
        return `
            <div class="chart-bar">
                <div class="chart-bar-label" title="${key}">${truncateText(key, 15)}</div>
                <div class="chart-bar-container">
                    <div class="chart-bar-fill" style="width: ${percentage}%"></div>
                </div>
                <div class="chart-bar-value">${displayValue}</div>
            </div>
        `;
    }).join('');
}

function renderAnalyticsTable(stats) {
    const tbody = document.getElementById('analytics-details-table');
    if (!tbody) return;
    
    const rows = [];
    
    // Provider rows
    const providers = Object.keys(stats.cost_by_provider || {});
    providers.forEach(provider => {
        rows.push({
            type: 'Provider',
            name: provider,
            requests: stats.requests_by_provider?.[provider] || 0,
            cost: stats.cost_by_provider?.[provider] || 0
        });
    });
    
    // User rows
    const users = Object.keys(stats.cost_by_user || {});
    users.forEach(user => {
        rows.push({
            type: 'User',
            name: user,
            requests: stats.requests_by_user?.[user] || 0,
            cost: stats.cost_by_user?.[user] || 0
        });
    });
    
    if (rows.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" style="text-align: center;">No data available</td></tr>';
        return;
    }
    
    tbody.innerHTML = rows.map(row => {
        const avgLatency = row.requests > 0 ? (stats.avg_latency_ms || 0) : 0;
        const tokens = Math.round((stats.total_tokens || 0) * (row.requests / (stats.total_requests || 1)));
        
        return `
            <tr>
                <td>${row.type}</td>
                <td><strong>${row.name}</strong></td>
                <td>${row.requests.toLocaleString()}</td>
                <td>${tokens.toLocaleString()}</td>
                <td>$${row.cost.toFixed(2)}</td>
                <td>${Math.round(avgLatency)}</td>
            </tr>
        `;
    }).join('');
}

function truncateText(text, maxLength) {
    if (!text) return '';
    return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
}

// Event listeners for analytics
document.getElementById('analytics-time-range')?.addEventListener('change', function() {
    const customRange = document.getElementById('analytics-custom-range');
    if (this.value === 'custom') {
        customRange.style.display = 'flex';
        customRange.style.gap = '0.5rem';
    } else {
        customRange.style.display = 'none';
        loadAnalytics();
    }
});

document.getElementById('refresh-analytics-btn')?.addEventListener('click', loadAnalytics);

// Load analytics when tab is shown
document.querySelectorAll('.view-tab').forEach(tab => {
    const originalClick = tab.onclick;
    tab.onclick = function() {
        if (originalClick) originalClick.call(this);
        if (this.dataset.target === 'analytics' && state.token) {
            setTimeout(loadAnalytics, 100);
        }
    };
});

// Export analytics data
async function exportAnalytics(type, format) {
    const timeRange = document.getElementById('analytics-time-range').value;
    let startTime, endTime;
    
    const now = new Date();
    endTime = now.toISOString();
    
    switch (timeRange) {
        case '1h':
            startTime = new Date(now - 60 * 60 * 1000).toISOString();
            break;
        case '24h':
            startTime = new Date(now - 24 * 60 * 60 * 1000).toISOString();
            break;
        case '7d':
            startTime = new Date(now - 7 * 24 * 60 * 60 * 1000).toISOString();
            break;
        case '30d':
            startTime = new Date(now - 30 * 24 * 60 * 60 * 1000).toISOString();
            break;
        case 'custom':
            startTime = document.getElementById('analytics-start-time').value;
            endTime = document.getElementById('analytics-end-time').value;
            break;
    }
    
    try {
        const params = new URLSearchParams();
        if (startTime) params.append('start_time', startTime);
        if (endTime) params.append('end_time', endTime);
        if (format) params.append('format', format);
        
        const endpoint = type === 'logs' ? '/api/v1/analytics/export' : '/api/v1/analytics/export-stats';
        const url = `${endpoint}?${params}`;
        
        // Create a temporary link to trigger download
        const a = document.createElement('a');
        a.href = url;
        a.setAttribute('download', '');
        a.style.display = 'none';
        
        // Add authorization header via fetch and create blob URL
        const response = await fetch(url, {
            headers: {
                'Authorization': `Bearer ${state.token}`
            }
        });
        
        if (!response.ok) {
            throw new Error('Export failed: ' + response.statusText);
        }
        
        const blob = await response.blob();
        const blobUrl = URL.createObjectURL(blob);
        a.href = blobUrl;
        
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        
        // Clean up blob URL after a short delay
        setTimeout(() => URL.revokeObjectURL(blobUrl), 100);
        
        showToast('Export started. Your download should begin shortly.', 'success');
    } catch (error) {
        console.error('Error exporting analytics:', error);
        showToast('Failed to export: ' + error.message, 'error');
    }
}

// Event listeners for export buttons
document.getElementById('export-stats-json-btn')?.addEventListener('click', () => {
    exportAnalytics('stats', 'json');
});

document.getElementById('export-stats-csv-btn')?.addEventListener('click', () => {
    exportAnalytics('stats', 'csv');
});

document.getElementById('export-logs-csv-btn')?.addEventListener('click', () => {
    exportAnalytics('logs', 'csv');
});

// =====================
// Motivations Dashboard
// =====================

let motivationsState = {
    motivations: [],
    roles: [],
    history: [],
    idleState: null
};

// Expose motivationsState to window for diagrams
window.motivationsState = motivationsState;

async function loadMotivations() {
    try {
        // Load all motivations data in parallel
        const [motivationsRes, rolesRes, historyRes, idleRes] = await Promise.all([
            fetch(`${API_BASE}/motivations`, { headers: getAuthHeaders() }),
            fetch(`${API_BASE}/motivations/roles`, { headers: getAuthHeaders() }),
            fetch(`${API_BASE}/motivations/history?limit=50`, { headers: getAuthHeaders() }),
            fetch(`${API_BASE}/motivations/idle`, { headers: getAuthHeaders() })
        ]);

        if (motivationsRes.ok) {
            const data = await motivationsRes.json();
            // API returns {count, motivations}, extract the array
            motivationsState.motivations = data.motivations || data || [];
        }
        if (rolesRes.ok) {
            const rolesData = await rolesRes.json();
            // API returns {motivations: {role: [...]}} - transform to {roles: [{role, motivations}]}
            if (rolesData.motivations && typeof rolesData.motivations === 'object') {
                motivationsState.roles = {
                    roles: Object.entries(rolesData.motivations).map(([role, motivations]) => ({
                        role,
                        motivations: motivations || []
                    }))
                };
            } else {
                motivationsState.roles = rolesData;
            }
            populateMotivationRoleFilter();
        }
        if (historyRes.ok) {
            const historyData = await historyRes.json();
            motivationsState.history = historyData.history || [];
        }
        if (idleRes.ok) {
            motivationsState.idleState = await idleRes.json();
        }

        renderMotivationsDashboard();
    } catch (error) {
        console.error('Error loading motivations:', error);
        showToast('Failed to load motivations: ' + error.message, 'error');
    }
}

function populateMotivationRoleFilter() {
    const select = document.getElementById('motivation-role-filter');
    if (!select || !motivationsState.roles) return;
    
    // Keep the "All Roles" option
    select.innerHTML = '<option value="">All Roles</option>';
    
    // Add role options
    const roles = motivationsState.roles.roles || [];
    roles.forEach(role => {
        const option = document.createElement('option');
        option.value = role.role;
        option.textContent = role.role.replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
        select.appendChild(option);
    });
}

function renderMotivationsDashboard() {
    renderMotivationStats();
    renderMotivationsByRole();
    renderMotivationsTable();
    renderMotivationHistory();
}

function renderMotivationStats() {
    // System idle status
    const idleEl = document.getElementById('motivation-system-idle');
    const idleLabelEl = document.getElementById('motivation-idle-label');
    if (idleEl && motivationsState.idleState) {
        const isIdle = motivationsState.idleState.system_idle;
        idleEl.textContent = isIdle ? '💤' : '✅';
        idleEl.style.color = isIdle ? 'var(--warning-color)' : 'var(--success-color)';
        idleLabelEl.textContent = isIdle ? 'System Idle' : 'Active';
    }

    // Total count
    const totalEl = document.getElementById('motivation-total-count');
    if (totalEl) {
        totalEl.textContent = motivationsState.motivations.length || 0;
    }

    // Active count
    const activeEl = document.getElementById('motivation-active-count');
    if (activeEl) {
        const activeCount = motivationsState.motivations.filter(m => m.status === 'active').length;
        activeEl.textContent = activeCount;
    }

    // Recent triggers (last 24h)
    const triggersEl = document.getElementById('motivation-recent-triggers');
    if (triggersEl) {
        const now = Date.now();
        const oneDayAgo = now - 24 * 60 * 60 * 1000;
        const recentCount = (motivationsState.history || []).filter(h => {
            const triggerTime = new Date(h.triggered_at).getTime();
            return triggerTime > oneDayAgo;
        }).length;
        triggersEl.textContent = recentCount;
    }
}

function renderMotivationsByRole() {
    const container = document.getElementById('motivations-by-role');
    if (!container || !motivationsState.roles) return;

    const roles = motivationsState.roles.roles || [];
    if (roles.length === 0) {
        container.innerHTML = '<p>No motivations registered.</p>';
        return;
    }

    container.innerHTML = roles.map(roleData => {
        const motivations = roleData.motivations || [];
        const activeCount = motivations.filter(m => m.status === 'active').length;
        
        return `
            <div class="motivation-role-card">
                <h4>${roleData.role.replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase())}</h4>
                <div class="motivation-role-stats">
                    <span class="stat">${motivations.length} total</span>
                    <span class="stat active">${activeCount} active</span>
                </div>
                <ul class="motivation-list">
                    ${motivations.slice(0, 3).map(m => `
                        <li class="${m.status}">
                            <span class="motivation-type type-${m.type}">${m.type}</span>
                            ${m.name}
                        </li>
                    `).join('')}
                    ${motivations.length > 3 ? `<li class="more">+${motivations.length - 3} more</li>` : ''}
                </ul>
            </div>
        `;
    }).join('');
}

function renderMotivationsTable() {
    const tbody = document.getElementById('motivations-table-body');
    if (!tbody) return;

    // Apply filters
    const typeFilter = document.getElementById('motivation-type-filter')?.value || '';
    const roleFilter = document.getElementById('motivation-role-filter')?.value || '';
    const statusFilter = document.getElementById('motivation-status-filter')?.value || '';

    let filtered = motivationsState.motivations;
    if (typeFilter) {
        filtered = filtered.filter(m => m.type === typeFilter);
    }
    if (roleFilter) {
        filtered = filtered.filter(m => m.agent_role === roleFilter);
    }
    if (statusFilter) {
        filtered = filtered.filter(m => m.status === statusFilter);
    }

    if (filtered.length === 0) {
        tbody.innerHTML = '<tr><td colspan="8" style="text-align: center;">No motivations found.</td></tr>';
        return;
    }

    tbody.innerHTML = filtered.map(m => {
        const cooldownMins = m.cooldown_minutes || Math.round((m.cooldown_period || 0) / 60000000000);
        const statusClass = m.status === 'active' ? 'success' : 'muted';
        const isBuiltIn = m.is_built_in || m.built_in;
        
        return `
            <tr>
                <td>
                    <strong>${escapeHtml(m.name)}</strong>
                    ${isBuiltIn ? '<span class="badge">built-in</span>' : ''}
                </td>
                <td><span class="motivation-type type-${m.type}">${m.type}</span></td>
                <td>${escapeHtml(m.condition)}</td>
                <td>${escapeHtml(m.agent_role || '-')}</td>
                <td>${m.priority || 50}</td>
                <td>${cooldownMins}m</td>
                <td><span class="status-${statusClass}">${m.status}</span></td>
                <td>
                    ${m.status === 'active' 
                        ? `<button class="small secondary" onclick="disableMotivation('${m.id}')">Disable</button>`
                        : `<button class="small primary" onclick="enableMotivation('${m.id}')">Enable</button>`
                    }
                    <button class="small" onclick="triggerMotivation('${m.id}')">Trigger</button>
                </td>
            </tr>
        `;
    }).join('');
}

function renderMotivationHistory() {
    const tbody = document.getElementById('motivation-history-table');
    if (!tbody) return;

    const history = motivationsState.history || [];
    if (history.length === 0) {
        tbody.innerHTML = '<tr><td colspan="4" style="text-align: center;">No trigger history.</td></tr>';
        return;
    }

    tbody.innerHTML = history.slice(0, 20).map(h => {
        const date = new Date(h.triggered_at);
        const timeAgo = formatTimeAgo(date);
        const resultClass = h.result === 'success' ? 'success' : (h.result === 'error' ? 'error' : 'muted');
        
        return `
            <tr>
                <td>${escapeHtml(h.motivation_name || h.motivation_id)}</td>
                <td>${escapeHtml(h.agent_role || '-')}</td>
                <td title="${date.toISOString()}">${timeAgo}</td>
                <td><span class="status-${resultClass}">${h.result || '-'}</span></td>
            </tr>
        `;
    }).join('');
}

function formatTimeAgo(date) {
    const now = new Date();
    const diffMs = now - date;
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);
    
    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return `${diffMins}m ago`;
    if (diffHours < 24) return `${diffHours}h ago`;
    return `${diffDays}d ago`;
}

async function enableMotivation(id) {
    try {
        const response = await fetch(`${API_BASE}/motivations/${id}/enable`, {
            method: 'POST',
            headers: getAuthHeaders()
        });
        if (!response.ok) throw new Error('Failed to enable motivation');
        showToast('Motivation enabled', 'success');
        loadMotivations();
    } catch (error) {
        showToast('Error: ' + error.message, 'error');
    }
}

async function disableMotivation(id) {
    try {
        const response = await fetch(`${API_BASE}/motivations/${id}/disable`, {
            method: 'POST',
            headers: getAuthHeaders()
        });
        if (!response.ok) throw new Error('Failed to disable motivation');
        showToast('Motivation disabled', 'success');
        loadMotivations();
    } catch (error) {
        showToast('Error: ' + error.message, 'error');
    }
}

async function triggerMotivation(id) {
    try {
        const response = await fetch(`${API_BASE}/motivations/${id}/trigger`, {
            method: 'POST',
            headers: getAuthHeaders()
        });
        if (!response.ok) throw new Error('Failed to trigger motivation');
        showToast('Motivation triggered manually', 'success');
        // Reload to see the new history entry
        setTimeout(loadMotivations, 1000);
    } catch (error) {
        showToast('Error: ' + error.message, 'error');
    }
}

// Motivation event listeners
document.getElementById('refresh-motivations-btn')?.addEventListener('click', loadMotivations);
document.getElementById('motivation-type-filter')?.addEventListener('change', renderMotivationsTable);
document.getElementById('motivation-role-filter')?.addEventListener('change', renderMotivationsTable);
document.getElementById('motivation-status-filter')?.addEventListener('change', renderMotivationsTable);

// --- Agent Conversation Viewer ---

let conversationAutoRefreshTimer = null;

function stopConversationAutoRefresh() {
    if (conversationAutoRefreshTimer) {
        clearInterval(conversationAutoRefreshTimer);
        conversationAutoRefreshTimer = null;
    }
}

async function viewAgentConversation(beadId) {
    if (!beadId || beadId === 'undefined' || beadId === 'null') {
        showToast('No bead ID available for this agent', 'error');
        return;
    }
    stopConversationAutoRefresh();

    const bodyHtml = `<div id="conv-container" class="conv-container"><div class="loading">Loading conversation...</div></div>`;
    openAppModal({ title: 'Agent Conversation', bodyHtml });

    // Make the modal wider for conversation view
    const modalContent = document.querySelector('#app-modal .modal-content');
    if (modalContent) modalContent.style.maxWidth = '900px';

    await refreshConversation(beadId);

    // Auto-refresh every 10s while modal is open
    conversationAutoRefreshTimer = setInterval(() => {
        if (modalState.activeId === 'app-modal') {
            refreshConversation(beadId);
        } else {
            stopConversationAutoRefresh();
        }
    }, 10000);
}

async function refreshConversation(beadId) {
    const container = document.getElementById('conv-container');
    if (!container) return;
    if (!beadId || beadId === 'undefined' || beadId === 'null') {
        container.innerHTML = '<div class="empty-state"><p>No bead ID provided.</p></div>';
        return;
    }

    try {
        const session = await apiCall(`/beads/${encodeURIComponent(beadId)}/conversation`, { suppressToast: true, skipAutoFile: true });
        if (!session || !session.messages || session.messages.length === 0) {
            container.innerHTML = '<div class="empty-state"><p>No conversation messages yet.</p></div>';
            return;
        }
        container.innerHTML = renderConversationMessages(session.messages);
        // Scroll to bottom
        container.scrollTop = container.scrollHeight;
    } catch (error) {
        if (container.innerHTML.includes('Loading')) {
            container.innerHTML = `<div class="empty-state"><p>No conversation found for this bead.</p></div>`;
        }
    }
}

function renderConversationMessages(messages) {
    return messages.map(msg => {
        const roleClass = msg.role === 'system' ? 'conv-system' :
                         msg.role === 'user' ? 'conv-user' : 'conv-assistant';
        const roleLabel = msg.role === 'system' ? 'System' :
                         msg.role === 'user' ? 'Feedback' : 'Agent';
        const time = msg.timestamp ? new Date(msg.timestamp).toLocaleTimeString() : '';

        return `
            <div class="conv-message ${roleClass}">
                <div class="conv-message-header">
                    <span class="conv-role">${roleLabel}</span>
                    ${time ? `<span class="conv-time">${time}</span>` : ''}
                </div>
                <div class="conv-message-body">${formatConversationContent(msg.content, msg.role)}</div>
            </div>
        `;
    }).join('');
}

function formatConversationContent(content, role) {
    if (!content) return '';
    let text = escapeHtml(content);

    // Truncate very long system prompts
    if (role === 'system' && text.length > 500) {
        const preview = text.substring(0, 500);
        return `<div class="conv-truncated">${preview.replace(/\n/g, '<br>')}...<br><small>[System prompt truncated]</small></div>`;
    }

    // For assistant messages, try to show JSON actions compactly
    if (role === 'assistant') {
        try {
            const parsed = JSON.parse(content);
            if (parsed.actions && Array.isArray(parsed.actions)) {
                const summary = parsed.actions.map(a => {
                    let detail = a.type;
                    if (a.path) detail += `: ${a.path}`;
                    else if (a.command) detail += `: ${a.command.substring(0, 60)}`;
                    else if (a.bead && a.bead.title) detail += `: ${a.bead.title}`;
                    return `<span class="conv-action-tag">${escapeHtml(detail)}</span>`;
                }).join(' ');
                let html = `<div class="conv-actions-summary">${summary}</div>`;
                if (parsed.thinking) {
                    html = `<div class="conv-thinking">${escapeHtml(parsed.thinking.substring(0, 300))}${parsed.thinking.length > 300 ? '...' : ''}</div>` + html;
                }
                return html;
            }
        } catch (e) { /* not JSON, render as text */ }
    }

    // Basic markdown formatting
    text = text.replace(/```([^`]+)```/g, '<pre class="conv-code">$1</pre>');
    text = text.replace(/`([^`]+)`/g, '<code>$1</code>');
    text = text.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    text = text.replace(/## ([^\n]+)/g, '<strong>$1</strong>');
    text = text.replace(/\n/g, '<br>');
    return text;
}
