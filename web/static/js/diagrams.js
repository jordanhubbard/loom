/**
 * AgentiCorp Workflow Diagrams
 *
 * Interactive diagram views using Cytoscape.js:
 * - Project Hierarchy: Projects → Agents → Beads
 * - Motivation Flow: Triggers → Roles → Actions
 * - Message Flow: Components → Events → Messages
 */

// API Base URL (defined in app.js, shared globally)

// Diagram state
let diagramState = {
    currentType: 'hierarchy', // 'hierarchy', 'motivation', 'message'
    currentInstance: null,
    projectFilter: 'all',
    autoRefresh: true
};

/**
 * Base class for diagram management
 */
class DiagramManager {
    constructor(containerId, type) {
        this.containerId = containerId;
        this.type = type;
        this.cy = null;
        this.data = null;
    }

    /**
     * Initialize Cytoscape instance
     */
    init(elements, layout, style) {
        const container = document.getElementById(this.containerId);
        if (!container) {
            console.error(`[Diagrams] Container ${this.containerId} not found`);
            return;
        }

        // Destroy existing instance if present
        if (this.cy) {
            this.cy.destroy();
        }

        try {
            this.cy = cytoscape({
                container: container,
                elements: elements,
                layout: layout,
                style: style,
                minZoom: 0.1,
                maxZoom: 3,
                wheelSensitivity: 0.2,
                userZoomingEnabled: true,
                userPanningEnabled: true,
                boxSelectionEnabled: true
            });

            // Add event handlers
            this.addEventHandlers();
        } catch (error) {
            console.error('[Diagrams] Failed to initialize Cytoscape:', error);
        }
    }

    /**
     * Add event handlers for node interactions
     */
    addEventHandlers() {
        if (!this.cy) return;

        // Node click handler
        this.cy.on('tap', 'node', (evt) => {
            const node = evt.target;
            const data = node.data();
            this.showNodeDetails(data);
        });

        // Double click to expand/collapse
        this.cy.on('dbltap', 'node', (evt) => {
            const node = evt.target;
            this.toggleNodeExpansion(node);
        });
    }

    /**
     * Show node details in a panel or modal
     */
    showNodeDetails(data) {
        console.log('[Diagrams] Node clicked:', data);
        // Could show a details panel or modal here
    }

    /**
     * Toggle node expansion (for hierarchical views)
     */
    toggleNodeExpansion(node) {
        // Placeholder for expand/collapse logic
        console.log('[Diagrams] Toggle expansion:', node.id());
    }

    /**
     * Export diagram as PNG
     */
    exportPNG() {
        if (!this.cy) {
            console.error('[Diagrams] No diagram to export');
            return;
        }

        const png = this.cy.png({
            full: true,
            scale: 2,
            bg: '#ffffff'
        });

        // Download the PNG
        const link = document.createElement('a');
        link.download = `agenticorp-${this.type}-diagram.png`;
        link.href = png;
        link.click();
    }

    /**
     * Export diagram as SVG
     */
    exportSVG() {
        if (!this.cy) {
            console.error('[Diagrams] No diagram to export');
            return;
        }

        const svg = this.cy.svg({
            full: true,
            scale: 1,
            bg: '#ffffff'
        });

        // Download the SVG
        const blob = new Blob([svg], { type: 'image/svg+xml' });
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.download = `agenticorp-${this.type}-diagram.svg`;
        link.href = url;
        link.click();
        URL.revokeObjectURL(url);
    }

    /**
     * Reset view to fit all nodes
     */
    resetView() {
        if (!this.cy) return;
        this.cy.fit(null, 50);
    }

    /**
     * Destroy the diagram instance
     */
    destroy() {
        if (this.cy) {
            this.cy.destroy();
            this.cy = null;
        }
    }
}

/**
 * Project Hierarchy Diagram
 * Shows: Projects → Agents → Beads
 */
class ProjectHierarchyDiagram extends DiagramManager {
    constructor(containerId) {
        super(containerId, 'hierarchy');
    }

    async loadData(projects, agents, beads, projectFilter = 'all') {
        // Filter by project if specified
        let filteredProjects = projects || [];
        if (projectFilter && projectFilter !== 'all') {
            filteredProjects = filteredProjects.filter(p => p.id === projectFilter);
        }

        const filteredAgents = (agents || []).filter(a =>
            projectFilter === 'all' || a.project_id === projectFilter
        );

        const filteredBeads = (beads || []).filter(b =>
            projectFilter === 'all' || b.project_id === projectFilter
        );

        this.data = {
            projects: filteredProjects,
            agents: filteredAgents,
            beads: filteredBeads
        };

        return this.data;
    }

    render() {
        if (!this.data) {
            console.error('[Diagrams] No data loaded for hierarchy diagram');
            return;
        }

        const elements = this.transformToElements(this.data);
        const layout = {
            name: 'breadthfirst',
            directed: true,
            spacingFactor: 1.5,
            padding: 50,
            animate: true,
            animationDuration: 500
        };
        const style = this.getHierarchyStyle();

        this.init(elements, layout, style);
        this.resetView();
    }

    transformToElements(data) {
        const nodes = [];
        const edges = [];

        // Add project nodes
        for (const project of data.projects) {
            nodes.push({
                data: {
                    id: `project-${project.id}`,
                    label: project.name || project.id,
                    type: 'project',
                    rawData: project
                }
            });
        }

        // Add agent nodes and edges
        for (const agent of data.agents) {
            nodes.push({
                data: {
                    id: `agent-${agent.id}`,
                    label: agent.name || agent.id,
                    type: 'agent',
                    status: agent.status || 'unknown',
                    rawData: agent
                }
            });

            // Connect agent to project
            if (agent.project_id) {
                edges.push({
                    data: {
                        source: `project-${agent.project_id}`,
                        target: `agent-${agent.id}`
                    }
                });
            }
        }

        // Add bead nodes and edges
        for (const bead of data.beads) {
            nodes.push({
                data: {
                    id: `bead-${bead.id}`,
                    label: bead.title || bead.id,
                    type: 'bead',
                    status: bead.status || 'unknown',
                    priority: bead.priority || 3,
                    rawData: bead
                }
            });

            // Connect bead to agent if assigned
            if (bead.assigned_to) {
                edges.push({
                    data: {
                        source: `agent-${bead.assigned_to}`,
                        target: `bead-${bead.id}`
                    }
                });
            } else if (bead.project_id) {
                // Connect orphan beads to their project
                edges.push({
                    data: {
                        source: `project-${bead.project_id}`,
                        target: `bead-${bead.id}`
                    }
                });
            }

            // Add dependency edges (blocked_by)
            if (bead.blocked_by && Array.isArray(bead.blocked_by)) {
                for (const depId of bead.blocked_by) {
                    edges.push({
                        data: {
                            source: `bead-${depId}`,
                            target: `bead-${bead.id}`,
                            type: 'dependency'
                        },
                        classes: 'dependency'
                    });
                }
            }
        }

        return { nodes, edges };
    }

    getHierarchyStyle() {
        return [
            // Default node style
            {
                selector: 'node',
                style: {
                    'label': 'data(label)',
                    'text-valign': 'center',
                    'text-halign': 'center',
                    'font-size': '12px',
                    'text-wrap': 'wrap',
                    'text-max-width': '80px',
                    'background-color': '#cbd5e1',
                    'border-width': 2,
                    'border-color': '#94a3b8',
                    'width': 60,
                    'height': 60
                }
            },
            // Project nodes
            {
                selector: 'node[type="project"]',
                style: {
                    'background-color': '#3b82f6',
                    'border-color': '#2563eb',
                    'shape': 'round-rectangle',
                    'width': 80,
                    'height': 60,
                    'font-weight': 'bold',
                    'color': '#ffffff',
                    'text-outline-color': '#1e40af',
                    'text-outline-width': 2
                }
            },
            // Agent nodes
            {
                selector: 'node[type="agent"]',
                style: {
                    'background-color': '#10b981',
                    'border-color': '#059669',
                    'shape': 'ellipse',
                    'width': 70,
                    'height': 70
                }
            },
            // Agent status colors
            {
                selector: 'node[type="agent"][status="idle"]',
                style: {
                    'background-color': '#94a3b8',
                    'border-color': '#64748b'
                }
            },
            {
                selector: 'node[type="agent"][status="working"]',
                style: {
                    'background-color': '#3b82f6',
                    'border-color': '#2563eb'
                }
            },
            {
                selector: 'node[type="agent"][status="blocked"]',
                style: {
                    'background-color': '#ef4444',
                    'border-color': '#dc2626'
                }
            },
            {
                selector: 'node[type="agent"][status="deciding"]',
                style: {
                    'background-color': '#f59e0b',
                    'border-color': '#d97706'
                }
            },
            // Bead nodes
            {
                selector: 'node[type="bead"]',
                style: {
                    'background-color': '#e5e7eb',
                    'border-color': '#9ca3af',
                    'shape': 'rectangle',
                    'width': 50,
                    'height': 40
                }
            },
            // Bead priority colors
            {
                selector: 'node[type="bead"][priority=0]',
                style: {
                    'background-color': '#fee2e2',
                    'border-color': '#dc2626',
                    'border-width': 3
                }
            },
            {
                selector: 'node[type="bead"][priority=1]',
                style: {
                    'background-color': '#fed7aa',
                    'border-color': '#f59e0b'
                }
            },
            {
                selector: 'node[type="bead"][priority=2]',
                style: {
                    'background-color': '#fef3c7',
                    'border-color': '#f59e0b'
                }
            },
            // Bead status
            {
                selector: 'node[type="bead"][status="closed"]',
                style: {
                    'background-color': '#d1fae5',
                    'border-color': '#10b981'
                }
            },
            {
                selector: 'node[type="bead"][status="in_progress"]',
                style: {
                    'background-color': '#dbeafe',
                    'border-color': '#3b82f6'
                }
            },
            // Default edges
            {
                selector: 'edge',
                style: {
                    'width': 2,
                    'line-color': '#94a3b8',
                    'target-arrow-color': '#94a3b8',
                    'target-arrow-shape': 'triangle',
                    'curve-style': 'bezier'
                }
            },
            // Dependency edges
            {
                selector: 'edge.dependency',
                style: {
                    'line-color': '#dc2626',
                    'target-arrow-color': '#dc2626',
                    'line-style': 'dashed',
                    'width': 1.5
                }
            },
            // Selected nodes
            {
                selector: ':selected',
                style: {
                    'border-width': 4,
                    'border-color': '#8b5cf6'
                }
            }
        ];
    }

    showNodeDetails(data) {
        let html = `<div class="diagram-node-details">`;
        html += `<h4>${escapeHtml(data.label)}</h4>`;
        html += `<p><strong>Type:</strong> ${escapeHtml(data.type)}</p>`;

        if (data.status) {
            html += `<p><strong>Status:</strong> ${escapeHtml(data.status)}</p>`;
        }

        if (data.priority !== undefined) {
            html += `<p><strong>Priority:</strong> P${data.priority}</p>`;
        }

        if (data.rawData && data.type === 'bead' && data.rawData.description) {
            html += `<p><strong>Description:</strong> ${escapeHtml(data.rawData.description.substring(0, 100))}...</p>`;
        }

        html += `</div>`;

        // Update details panel
        const detailsPanel = document.getElementById('diagram-details');
        if (detailsPanel) {
            detailsPanel.innerHTML = html;
            detailsPanel.style.display = 'block';
        }
    }
}

/**
 * Motivation Flow Diagram
 * Shows: Triggers → Roles → Actions
 */
class MotivationFlowDiagram extends DiagramManager {
    constructor(containerId) {
        super(containerId, 'motivation');
    }

    async loadData(motivations, agents) {
        this.data = {
            motivations: motivations || [],
            agents: agents || []
        };
        return this.data;
    }

    render() {
        if (!this.data) {
            console.error('[Diagrams] No data loaded for motivation diagram');
            return;
        }

        const elements = this.transformToElements(this.data);
        const layout = {
            name: 'dagre',
            rankDir: 'LR',
            padding: 50,
            animate: true,
            animationDuration: 500
        };
        const style = this.getMotivationStyle();

        this.init(elements, layout, style);
        this.resetView();
    }

    transformToElements(data) {
        const nodes = [];
        const edges = [];
        const roleNodes = new Set();

        // Add motivation nodes
        for (const motivation of data.motivations) {
            nodes.push({
                data: {
                    id: `motivation-${motivation.id}`,
                    label: motivation.name || `Motivation ${motivation.id}`,
                    type: 'motivation',
                    motivationType: motivation.type || 'unknown',
                    priority: motivation.priority || 3,
                    rawData: motivation
                }
            });

            // Create or link to role node
            const role = motivation.agent_role || 'unknown';
            const roleNodeId = `role-${role}`;

            if (!roleNodes.has(roleNodeId)) {
                nodes.push({
                    data: {
                        id: roleNodeId,
                        label: role,
                        type: 'role'
                    }
                });
                roleNodes.add(roleNodeId);
            }

            // Connect motivation to role
            edges.push({
                data: {
                    source: `motivation-${motivation.id}`,
                    target: roleNodeId,
                    label: motivation.cooldown_seconds ? `cooldown: ${motivation.cooldown_seconds}s` : ''
                }
            });

            // Add action node
            if (motivation.action) {
                const actionNodeId = `action-${motivation.id}`;
                nodes.push({
                    data: {
                        id: actionNodeId,
                        label: motivation.action.type || 'action',
                        type: 'action',
                        rawData: motivation.action
                    }
                });

                edges.push({
                    data: {
                        source: roleNodeId,
                        target: actionNodeId
                    }
                });
            }
        }

        return { nodes, edges };
    }

    getMotivationStyle() {
        return [
            // Default node style
            {
                selector: 'node',
                style: {
                    'label': 'data(label)',
                    'text-valign': 'center',
                    'text-halign': 'center',
                    'font-size': '11px',
                    'text-wrap': 'wrap',
                    'text-max-width': '80px',
                    'background-color': '#cbd5e1',
                    'border-width': 2,
                    'border-color': '#94a3b8',
                    'width': 60,
                    'height': 60
                }
            },
            // Motivation nodes
            {
                selector: 'node[type="motivation"]',
                style: {
                    'background-color': '#f59e0b',
                    'border-color': '#d97706',
                    'shape': 'diamond',
                    'width': 70,
                    'height': 70
                }
            },
            // Motivation type colors
            {
                selector: 'node[type="motivation"][motivationType="calendar"]',
                style: {
                    'background-color': '#3b82f6',
                    'border-color': '#2563eb'
                }
            },
            {
                selector: 'node[type="motivation"][motivationType="event"]',
                style: {
                    'background-color': '#8b5cf6',
                    'border-color': '#7c3aed'
                }
            },
            {
                selector: 'node[type="motivation"][motivationType="threshold"]',
                style: {
                    'background-color': '#ef4444',
                    'border-color': '#dc2626'
                }
            },
            {
                selector: 'node[type="motivation"][motivationType="idle"]',
                style: {
                    'background-color': '#94a3b8',
                    'border-color': '#64748b'
                }
            },
            // Role nodes
            {
                selector: 'node[type="role"]',
                style: {
                    'background-color': '#10b981',
                    'border-color': '#059669',
                    'shape': 'ellipse',
                    'width': 80,
                    'height': 60,
                    'font-weight': 'bold'
                }
            },
            // Action nodes
            {
                selector: 'node[type="action"]',
                style: {
                    'background-color': '#ec4899',
                    'border-color': '#db2777',
                    'shape': 'round-rectangle',
                    'width': 70,
                    'height': 50
                }
            },
            // Edges
            {
                selector: 'edge',
                style: {
                    'width': 2,
                    'line-color': '#94a3b8',
                    'target-arrow-color': '#94a3b8',
                    'target-arrow-shape': 'triangle',
                    'curve-style': 'bezier',
                    'label': 'data(label)',
                    'font-size': '9px',
                    'text-rotation': 'autorotate',
                    'text-margin-y': -10
                }
            },
            // Selected
            {
                selector: ':selected',
                style: {
                    'border-width': 4,
                    'border-color': '#8b5cf6'
                }
            }
        ];
    }

    showNodeDetails(data) {
        let html = `<div class="diagram-node-details">`;
        html += `<h4>${escapeHtml(data.label)}</h4>`;
        html += `<p><strong>Type:</strong> ${escapeHtml(data.type)}</p>`;

        if (data.motivationType) {
            html += `<p><strong>Motivation Type:</strong> ${escapeHtml(data.motivationType)}</p>`;
        }

        if (data.priority !== undefined) {
            html += `<p><strong>Priority:</strong> P${data.priority}</p>`;
        }

        html += `</div>`;

        const detailsPanel = document.getElementById('diagram-details');
        if (detailsPanel) {
            detailsPanel.innerHTML = html;
            detailsPanel.style.display = 'block';
        }
    }
}

/**
 * Message Flow Diagram
 * Shows: Components → Events → Messages (from logs)
 */
class MessageFlowDiagram extends DiagramManager {
    constructor(containerId) {
        super(containerId, 'message');
    }

    async loadData(logs) {
        this.data = {
            logs: logs || []
        };
        return this.data;
    }

    render() {
        if (!this.data) {
            console.error('[Diagrams] No data loaded for message flow diagram');
            return;
        }

        const elements = this.transformToElements(this.data);
        const layout = {
            name: 'cose',
            nodeRepulsion: 8000,
            idealEdgeLength: 100,
            padding: 50,
            animate: true,
            animationDuration: 500
        };
        const style = this.getMessageFlowStyle();

        this.init(elements, layout, style);
        this.resetView();
    }

    transformToElements(data) {
        const nodes = [];
        const edges = [];
        const componentNodes = new Set();
        const eventCounts = new Map();

        // Analyze logs to extract message flow patterns
        for (const log of data.logs) {
            if (!log.source) continue;

            const componentId = `component-${log.source}`;

            // Add component node
            if (!componentNodes.has(componentId)) {
                nodes.push({
                    data: {
                        id: componentId,
                        label: log.source,
                        type: 'component'
                    }
                });
                componentNodes.add(componentId);
            }

            // Extract event/message type from log
            const eventType = this.extractEventType(log);
            if (eventType) {
                const edgeKey = `${log.source}-${eventType}`;
                eventCounts.set(edgeKey, (eventCounts.get(edgeKey) || 0) + 1);
            }
        }

        // Add event nodes and edges based on counts
        for (const [edgeKey, count] of eventCounts.entries()) {
            const [source, event] = edgeKey.split('-', 2);
            const eventNodeId = `event-${event}`;

            // Add event node if not exists
            if (!nodes.find(n => n.data.id === eventNodeId)) {
                nodes.push({
                    data: {
                        id: eventNodeId,
                        label: event,
                        type: 'event',
                        count: count
                    }
                });
            }

            // Add edge
            edges.push({
                data: {
                    source: `component-${source}`,
                    target: eventNodeId,
                    weight: count,
                    label: `${count}`
                }
            });
        }

        return { nodes, edges };
    }

    extractEventType(log) {
        // Extract event type from log message
        const message = log.message || '';

        // Common patterns
        if (message.includes('bead')) return 'bead_event';
        if (message.includes('agent')) return 'agent_event';
        if (message.includes('workflow')) return 'workflow_event';
        if (message.includes('motivation')) return 'motivation_event';
        if (message.includes('dispatch')) return 'dispatch_event';
        if (message.includes('provider')) return 'provider_event';
        if (message.includes('error')) return 'error';
        if (message.includes('start')) return 'start';
        if (message.includes('complete')) return 'complete';

        return 'other';
    }

    getMessageFlowStyle() {
        return [
            // Default node style
            {
                selector: 'node',
                style: {
                    'label': 'data(label)',
                    'text-valign': 'center',
                    'text-halign': 'center',
                    'font-size': '11px',
                    'text-wrap': 'wrap',
                    'text-max-width': '70px',
                    'background-color': '#cbd5e1',
                    'border-width': 2,
                    'border-color': '#94a3b8',
                    'width': 60,
                    'height': 60
                }
            },
            // Component nodes
            {
                selector: 'node[type="component"]',
                style: {
                    'background-color': '#6366f1',
                    'border-color': '#4f46e5',
                    'shape': 'round-rectangle',
                    'width': 80,
                    'height': 60,
                    'font-weight': 'bold'
                }
            },
            // Event nodes
            {
                selector: 'node[type="event"]',
                style: {
                    'background-color': '#14b8a6',
                    'border-color': '#0d9488',
                    'shape': 'ellipse',
                    'width': 'mapData(count, 1, 100, 40, 80)',
                    'height': 'mapData(count, 1, 100, 40, 80)'
                }
            },
            // Edges
            {
                selector: 'edge',
                style: {
                    'width': 'mapData(weight, 1, 100, 1, 8)',
                    'line-color': '#94a3b8',
                    'target-arrow-color': '#94a3b8',
                    'target-arrow-shape': 'triangle',
                    'curve-style': 'bezier',
                    'label': 'data(label)',
                    'font-size': '9px',
                    'text-rotation': 'autorotate'
                }
            },
            // Selected
            {
                selector: ':selected',
                style: {
                    'border-width': 4,
                    'border-color': '#8b5cf6'
                }
            }
        ];
    }

    showNodeDetails(data) {
        let html = `<div class="diagram-node-details">`;
        html += `<h4>${escapeHtml(data.label)}</h4>`;
        html += `<p><strong>Type:</strong> ${escapeHtml(data.type)}</p>`;

        if (data.count) {
            html += `<p><strong>Count:</strong> ${data.count}</p>`;
        }

        html += `</div>`;

        const detailsPanel = document.getElementById('diagram-details');
        if (detailsPanel) {
            detailsPanel.innerHTML = html;
            detailsPanel.style.display = 'block';
        }
    }
}

/**
 * Initialize diagrams UI
 */
function initDiagramsUI() {
    // Register Cytoscape dagre layout extension
    if (typeof cytoscape !== 'undefined' && typeof cytoscapeDagre !== 'undefined') {
        cytoscape.use(cytoscapeDagre);
        console.log('[Diagrams] Cytoscape dagre extension registered');
    }

    // Diagram type selector
    const typeSelect = document.getElementById('diagram-type-select');
    if (typeSelect) {
        typeSelect.addEventListener('change', (e) => {
            diagramState.currentType = e.target.value;
            renderCurrentDiagram();
        });
    }

    // Project filter
    const projectFilter = document.getElementById('diagram-project-filter');
    if (projectFilter) {
        projectFilter.addEventListener('change', (e) => {
            diagramState.projectFilter = e.target.value;
            renderCurrentDiagram();
        });
    }

    // Export buttons
    const exportPngBtn = document.getElementById('diagram-export-png');
    if (exportPngBtn) {
        exportPngBtn.addEventListener('click', () => {
            if (diagramState.currentInstance) {
                diagramState.currentInstance.exportPNG();
            }
        });
    }

    const exportSvgBtn = document.getElementById('diagram-export-svg');
    if (exportSvgBtn) {
        exportSvgBtn.addEventListener('click', () => {
            if (diagramState.currentInstance) {
                diagramState.currentInstance.exportSVG();
            }
        });
    }

    // Reset view button
    const resetBtn = document.getElementById('diagram-reset-view');
    if (resetBtn) {
        resetBtn.addEventListener('click', () => {
            if (diagramState.currentInstance) {
                diagramState.currentInstance.resetView();
            }
        });
    }

    // Auto-refresh toggle
    const autoRefreshCheckbox = document.getElementById('diagram-auto-refresh');
    if (autoRefreshCheckbox) {
        autoRefreshCheckbox.addEventListener('change', (e) => {
            diagramState.autoRefresh = e.target.checked;
        });
    }

    // Listen for tab activation to render diagram
    const diagramsTab = document.querySelector('.view-tab[data-target="diagrams"]');
    if (diagramsTab) {
        diagramsTab.addEventListener('click', () => {
            // Small delay to ensure the panel is visible before rendering
            setTimeout(() => {
                if (!diagramState.currentInstance) {
                    renderCurrentDiagram();
                }
            }, 100);
        });
    }

    console.log('[Diagrams] UI initialized');
}

/**
 * Render the currently selected diagram
 */
async function renderCurrentDiagram() {
    console.log(`[Diagrams] Rendering ${diagramState.currentType} diagram`);

    // Destroy previous instance
    if (diagramState.currentInstance) {
        diagramState.currentInstance.destroy();
        diagramState.currentInstance = null;
    }

    // Clear details panel
    const detailsPanel = document.getElementById('diagram-details');
    if (detailsPanel) {
        detailsPanel.innerHTML = '';
        detailsPanel.style.display = 'none';
    }

    try {
        switch (diagramState.currentType) {
            case 'hierarchy':
                await renderHierarchyDiagram();
                break;
            case 'motivation':
                await renderMotivationDiagram();
                break;
            case 'message':
                await renderMessageFlowDiagram();
                break;
            default:
                console.error('[Diagrams] Unknown diagram type:', diagramState.currentType);
        }
    } catch (error) {
        console.error('[Diagrams] Failed to render diagram:', error);
        showDiagramError(error.message);
    }
}

/**
 * Render project hierarchy diagram
 */
async function renderHierarchyDiagram() {
    // Access global state from app.js
    if (!window.state) {
        throw new Error('Application state not available');
    }

    const diagram = new ProjectHierarchyDiagram('diagram-canvas');
    await diagram.loadData(
        window.state.projects,
        window.state.agents,
        window.state.beads,
        diagramState.projectFilter
    );
    diagram.render();
    diagramState.currentInstance = diagram;
}

/**
 * Render motivation flow diagram
 */
async function renderMotivationDiagram() {
    // Access global state from app.js
    if (!window.state || !window.motivationsState) {
        throw new Error('Application state not available');
    }

    const diagram = new MotivationFlowDiagram('diagram-canvas');
    await diagram.loadData(
        window.motivationsState.motivations,
        window.state.agents
    );
    diagram.render();
    diagramState.currentInstance = diagram;
}

/**
 * Render message flow diagram
 */
async function renderMessageFlowDiagram() {
    // Fetch recent logs
    let logs = [];
    try {
        const response = await fetch(`${API_BASE}/logs/recent?limit=200`);
        if (response.ok) {
            logs = await response.json();
        }
    } catch (error) {
        console.error('[Diagrams] Failed to fetch logs:', error);
    }

    const diagram = new MessageFlowDiagram('diagram-canvas');
    await diagram.loadData(logs);
    diagram.render();
    diagramState.currentInstance = diagram;
}

/**
 * Show error message in diagram area
 */
function showDiagramError(message) {
    const canvas = document.getElementById('diagram-canvas');
    if (canvas) {
        canvas.innerHTML = `
            <div style="display: flex; align-items: center; justify-content: center; height: 100%; color: var(--danger-color);">
                <div style="text-align: center;">
                    <h3>Error Loading Diagram</h3>
                    <p>${escapeHtml(message)}</p>
                    <p class="small">Check console for details</p>
                </div>
            </div>
        `;
    }
}

/**
 * Update diagram data (called by auto-refresh)
 */
function updateDiagramData() {
    if (diagramState.autoRefresh && diagramState.currentInstance) {
        renderCurrentDiagram();
    }
}

// Make functions available globally
window.initDiagramsUI = initDiagramsUI;
window.renderCurrentDiagram = renderCurrentDiagram;
window.updateDiagramData = updateDiagramData;
