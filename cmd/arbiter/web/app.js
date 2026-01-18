// API base URL
const API_BASE = '/api';

// Global state
let currentTab = 'all';

// Initialize app
document.addEventListener('DOMContentLoaded', () => {
    setupEventListeners();
    loadAllData();
    
    // Auto-refresh every 5 seconds
    setInterval(loadAllData, 5000);
});

function setupEventListeners() {
    // Work form submission
    document.getElementById('workForm').addEventListener('submit', async (e) => {
        e.preventDefault();
        await createWork();
    });
    
    // Tab switching
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            currentTab = btn.dataset.tab;
            loadServices();
        });
    });
    
    // Modal close
    document.querySelector('.close').addEventListener('click', closeModal);
    window.addEventListener('click', (e) => {
        const modal = document.getElementById('costModal');
        if (e.target === modal) {
            closeModal();
        }
    });
    
    // Cost type selection
    document.getElementById('costType').addEventListener('change', (e) => {
        const isFixed = e.target.value === 'fixed';
        document.getElementById('fixedCostGroup').style.display = isFixed ? 'block' : 'none';
        document.getElementById('costPerTokenGroup').style.display = isFixed ? 'none' : 'block';
    });
    
    // Cost form submission
    document.getElementById('costForm').addEventListener('submit', async (e) => {
        e.preventDefault();
        await updateServiceCosts();
    });
}

async function loadAllData() {
    await Promise.all([
        loadWork(),
        loadAgents(),
        loadServices()
    ]);
}

async function createWork() {
    const description = document.getElementById('workDescription').value;
    
    try {
        const response = await fetch(`${API_BASE}/work/create`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ description })
        });
        
        if (response.ok) {
            document.getElementById('workDescription').value = '';
            await loadWork();
            showNotification('Work created successfully!', 'success');
        } else {
            throw new Error('Failed to create work');
        }
    } catch (error) {
        console.error('Error creating work:', error);
        showNotification('Failed to create work', 'error');
    }
}

async function loadWork() {
    try {
        const response = await fetch(`${API_BASE}/work?status=in_progress`);
        const works = await response.json();
        
        const container = document.getElementById('workList');
        
        if (!works || works.length === 0) {
            container.innerHTML = '<p class="empty-state">No work in progress</p>';
            return;
        }
        
        container.innerHTML = works.map(work => `
            <div class="work-item">
                <div>
                    <strong>${escapeHtml(work.description)}</strong>
                    <span class="status-badge status-${work.status}">${work.status}</span>
                </div>
                <div style="margin-top: 8px; font-size: 0.9em; color: #666;">
                    ID: ${work.id}
                    ${work.assigned_to ? ` | Assigned to: ${work.assigned_to}` : ''}
                </div>
                <div style="margin-top: 4px; font-size: 0.85em; color: #999;">
                    Created: ${formatDate(work.created_at)}
                </div>
            </div>
        `).join('');
    } catch (error) {
        console.error('Error loading work:', error);
        document.getElementById('workList').innerHTML = '<p class="empty-state">Error loading work</p>';
    }
}

async function loadAgents() {
    try {
        const response = await fetch(`${API_BASE}/agents`);
        const data = await response.json();
        
        const agentsContainer = document.getElementById('agentsList');
        const commsContainer = document.getElementById('communicationsList');
        
        // Display agents
        if (!data.agents || data.agents.length === 0) {
            agentsContainer.innerHTML = '<p class="empty-state">No agents active</p>';
        } else {
            agentsContainer.innerHTML = data.agents.map(agent => `
                <div class="agent-item">
                    <div>
                        <span class="active-indicator ${agent.status === 'active' ? 'active' : 'inactive'}"></span>
                        <strong>${escapeHtml(agent.name)}</strong>
                        <span class="status-badge">${agent.status}</span>
                    </div>
                    <div style="margin-top: 8px; font-size: 0.9em; color: #666;">
                        ID: ${agent.id} | Service: ${agent.service_id || 'None'}
                    </div>
                    ${agent.current_work ? `<div style="margin-top: 4px; font-size: 0.85em; color: #999;">Working on: ${agent.current_work}</div>` : ''}
                </div>
            `).join('');
        }
        
        // Display communications
        if (!data.communications || data.communications.length === 0) {
            commsContainer.innerHTML = '<p class="empty-state">No communications yet</p>';
        } else {
            commsContainer.innerHTML = data.communications.slice(-10).reverse().map(comm => `
                <div class="comm-item">
                    <div class="comm-header">${comm.from_agent} → ${comm.to_agent}</div>
                    <div>${escapeHtml(comm.message)}</div>
                    <div class="comm-time">${formatDate(comm.timestamp)}</div>
                </div>
            `).join('');
        }
    } catch (error) {
        console.error('Error loading agents:', error);
        document.getElementById('agentsList').innerHTML = '<p class="empty-state">Error loading agents</p>';
    }
}

async function loadServices() {
    try {
        let url = `${API_BASE}/services`;
        
        if (currentTab === 'active') {
            url += '?active=true';
        } else if (currentTab === 'preferred') {
            url = `${API_BASE}/services/preferred`;
        }
        
        const response = await fetch(url);
        const services = await response.json();
        
        const container = document.getElementById('servicesList');
        
        if (!services || services.length === 0) {
            container.innerHTML = '<p class="empty-state">No services found</p>';
            return;
        }
        
        container.innerHTML = services.map(service => {
            const costTypeClass = service.cost_type === 'fixed' ? 'fixed-cost' : 'variable-cost';
            const activeClass = service.is_active ? '' : 'inactive';
            const fixedCost = service.fixed_cost || 0;
            const costPerToken = service.cost_per_token || 0;
            const costDisplay = service.cost_type === 'fixed' 
                ? `Fixed: $${fixedCost.toFixed(2)}`
                : `Per token: $${costPerToken.toFixed(8)}`;
            
            return `
                <div class="service-item ${costTypeClass} ${activeClass}" onclick="openCostModal('${service.id}', '${escapeHtml(service.name)}', '${service.cost_type}', ${costPerToken}, ${fixedCost})">
                    <div>
                        <span class="active-indicator ${service.is_active ? 'active' : 'inactive'}"></span>
                        <strong>${escapeHtml(service.name)}</strong>
                        <span class="cost-badge cost-${service.cost_type}">${service.cost_type}</span>
                    </div>
                    <div style="margin-top: 8px; font-size: 0.9em; color: #666;">
                        ${service.type} | ${service.url}
                    </div>
                    <div class="service-stats">
                        <div class="stat">
                            <span class="stat-label">Cost Model</span>
                            <span class="stat-value">${costDisplay}</span>
                        </div>
                        <div class="stat">
                            <span class="stat-label">Tokens Used</span>
                            <span class="stat-value">${formatNumber(service.tokens_used)}</span>
                        </div>
                        <div class="stat">
                            <span class="stat-label">Total Cost</span>
                            <span class="stat-value">$${service.total_cost.toFixed(4)}</span>
                        </div>
                        <div class="stat">
                            <span class="stat-label">Requests</span>
                            <span class="stat-value">${service.request_count}</span>
                        </div>
                    </div>
                    ${service.is_active ? `<div style="margin-top: 8px; font-size: 0.85em; color: #999;">Last active: ${formatDate(service.last_active)}</div>` : ''}
                    <div style="margin-top: 8px; font-size: 0.85em; color: #667eea; font-weight: 600;">
                        Click to edit costs
                    </div>
                </div>
            `;
        }).join('');
    } catch (error) {
        console.error('Error loading services:', error);
        document.getElementById('servicesList').innerHTML = '<p class="empty-state">Error loading services</p>';
    }
}

function openCostModal(serviceId, serviceName, costType, costPerToken, fixedCost) {
    document.getElementById('modalServiceId').value = serviceId;
    document.getElementById('modalServiceName').textContent = serviceName;
    document.getElementById('costType').value = costType;
    document.getElementById('costPerToken').value = costPerToken;
    document.getElementById('fixedCost').value = fixedCost;
    
    const isFixed = costType === 'fixed';
    document.getElementById('fixedCostGroup').style.display = isFixed ? 'block' : 'none';
    document.getElementById('costPerTokenGroup').style.display = isFixed ? 'none' : 'block';
    
    document.getElementById('costModal').style.display = 'block';
}

function closeModal() {
    document.getElementById('costModal').style.display = 'none';
}

async function updateServiceCosts() {
    const serviceId = document.getElementById('modalServiceId').value;
    const costType = document.getElementById('costType').value;
    const costPerToken = parseFloat(document.getElementById('costPerToken').value) || 0;
    const fixedCost = parseFloat(document.getElementById('fixedCost').value) || 0;
    
    const body = {
        cost_type: costType
    };
    
    if (costType === 'fixed') {
        body.fixed_cost = fixedCost;
    } else {
        body.cost_per_token = costPerToken;
    }
    
    try {
        const response = await fetch(`${API_BASE}/services/${serviceId}/costs`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
        });
        
        if (response.ok) {
            closeModal();
            await loadServices();
            showNotification('Service costs updated successfully!', 'success');
        } else {
            throw new Error('Failed to update costs');
        }
    } catch (error) {
        console.error('Error updating costs:', error);
        showNotification('Failed to update service costs', 'error');
    }
}

// Utility functions
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function formatDate(dateString) {
    const date = new Date(dateString);
    return date.toLocaleString();
}

function formatNumber(num) {
    return num.toLocaleString();
}

function showNotification(message, type) {
    // Simple notification - could be enhanced with a proper notification library
    const notification = document.createElement('div');
    notification.textContent = message;
    notification.style.cssText = `
        position: fixed;
        top: 20px;
        right: 20px;
        padding: 15px 25px;
        background: ${type === 'success' ? '#28a745' : '#dc3545'};
        color: white;
        border-radius: 8px;
        box-shadow: 0 4px 12px rgba(0,0,0,0.3);
        z-index: 10000;
        animation: slideIn 0.3s;
    `;
    
    document.body.appendChild(notification);
    
    setTimeout(() => {
        notification.style.animation = 'slideOut 0.3s';
        setTimeout(() => notification.remove(), 300);
    }, 3000);
}
