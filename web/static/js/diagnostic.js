// AgentiCorp UI Self-Diagnostic Tool
// This script runs diagnostics and reports UI issues automatically

(function() {
    'use strict';
    
    const diagnostics = {
        errors: [],
        warnings: [],
        info: []
    };
    
    // Run diagnostics after page load
    window.addEventListener('load', () => {
        setTimeout(runDiagnostics, 2000); // Wait 2 seconds for everything to settle
    });
    
    function runDiagnostics() {
        console.log('[Diagnostic] Running UI diagnostics...');
        
        // Check 1: Verify state is populated
        if (typeof state === 'undefined') {
            diagnostics.errors.push('Global state object not defined');
        } else {
            if (!state.beads || state.beads.length === 0) {
                diagnostics.warnings.push(`Beads not loaded (state.beads = ${state.beads?.length || 0})`);
            } else {
                diagnostics.info.push(`✓ ${state.beads.length} beads loaded`);
            }
            
            if (!state.projects || state.projects.length === 0) {
                diagnostics.warnings.push(`Projects not loaded (state.projects = ${state.projects?.length || 0})`);
            } else {
                diagnostics.info.push(`✓ ${state.projects.length} projects loaded`);
            }
            
            if (!state.agents || state.agents.length === 0) {
                diagnostics.warnings.push(`Agents not loaded (state.agents = ${state.agents?.length || 0})`);
            } else {
                diagnostics.info.push(`✓ ${state.agents.length} agents loaded`);
            }
        }
        
        // Check 2: Verify DOM elements exist
        const requiredElements = [
            'project-view-select',
            'project-view-details',
            'open-beads',
            'in-progress-beads',
            'closed-beads'
        ];
        
        requiredElements.forEach(id => {
            const el = document.getElementById(id);
            if (!el) {
                diagnostics.errors.push(`Missing required DOM element: #${id}`);
            } else if (el.innerHTML.trim() === '') {
                diagnostics.warnings.push(`DOM element #${id} is empty`);
            } else {
                diagnostics.info.push(`✓ Element #${id} exists and has content`);
            }
        });
        
        // Check 3: Look for JavaScript errors in console
        const consoleErrors = window._agenticorpErrors || [];
        if (consoleErrors.length > 0) {
            diagnostics.errors.push(`${consoleErrors.length} JavaScript errors detected`);
        }
        
        // Check 4: Verify API responses
        if (typeof fetch !== 'undefined') {
            checkAPIEndpoints();
        }
        
        // Display results
        displayDiagnostics();
    }
    
    async function checkAPIEndpoints() {
        const endpoints = ['/api/v1/projects', '/api/v1/beads', '/api/v1/agents'];
        
        for (const endpoint of endpoints) {
            try {
                const response = await fetch(endpoint);
                if (!response.ok) {
                    diagnostics.warnings.push(`API ${endpoint} returned ${response.status}`);
                } else {
                    const data = await response.json();
                    diagnostics.info.push(`✓ API ${endpoint} returned ${Array.isArray(data) ? data.length : '?'} items`);
                }
            } catch (error) {
                diagnostics.errors.push(`API ${endpoint} failed: ${error.message}`);
            }
        }
    }
    
    function displayDiagnostics() {
        console.log('\n╔════════════════════════════════════════════════════════════════╗');
        console.log('║  AgentiCorp UI Diagnostics Report                             ║');
        console.log('╚════════════════════════════════════════════════════════════════╝\n');
        
        if (diagnostics.errors.length > 0) {
            console.error('❌ ERRORS:');
            diagnostics.errors.forEach(err => console.error('   •', err));
            console.log('');
        }
        
        if (diagnostics.warnings.length > 0) {
            console.warn('⚠️  WARNINGS:');
            diagnostics.warnings.forEach(warn => console.warn('   •', warn));
            console.log('');
        }
        
        if (diagnostics.info.length > 0) {
            console.log('✓ INFO:');
            diagnostics.info.forEach(info => console.log('   •', info));
            console.log('');
        }
        
        // Generate report summary
        const totalIssues = diagnostics.errors.length + diagnostics.warnings.length;
        if (totalIssues === 0) {
            console.log('%c✅ No issues detected - UI should be working!', 'color: green; font-weight: bold');
        } else {
            console.log(`%c⚠️  Found ${totalIssues} issues`, 'color: orange; font-weight: bold');
            
            // Suggest fixes
            if (diagnostics.errors.some(e => e.includes('DOM element'))) {
                console.log('\n💡 Suggestion: HTML structure may be incomplete. Check index.html.');
            }
            if (diagnostics.warnings.some(w => w.includes('not loaded'))) {
                console.log('\n💡 Suggestion: Data loaded but state not populated. Check loadAll() function.');
            }
            if (diagnostics.warnings.some(w => w.includes('empty'))) {
                console.log('\n💡 Suggestion: Data exists but not rendering. Check render() functions.');
            }
        }
        
        console.log('\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n');
        
        // Auto-file bug if critical errors found
        if (diagnostics.errors.length > 0) {
            autoFileBugReport();
        }
    }
    
    // Capture console errors
    window._agenticorpErrors = [];
    const originalError = console.error;
    console.error = function(...args) {
        window._agenticorpErrors.push(args.join(' '));
        originalError.apply(console, args);
    };
    
    // Auto-file bug report to backend
    async function autoFileBugReport() {
        const errorSummary = diagnostics.errors.slice(0, 3).join('; ');
        const title = `UI Error: ${errorSummary.substring(0, 100)}`;
        
        const bugReport = {
            title: title,
            source: 'frontend',
            error_type: 'ui_error',
            message: diagnostics.errors.join('\n'),
            stack_trace: window._agenticorpErrors.join('\n'),
            context: {
                url: window.location.href,
                user_agent: navigator.userAgent,
                timestamp: new Date().toISOString(),
                viewport: `${window.innerWidth}x${window.innerHeight}`,
                warnings: diagnostics.warnings,
                state: typeof state !== 'undefined' ? {
                    beads: state.beads?.length || 0,
                    projects: state.projects?.length || 0,
                    agents: state.agents?.length || 0,
                    providers: state.providers?.length || 0
                } : 'undefined'
            },
            severity: diagnostics.errors.length > 2 ? 'critical' : 'high',
            occurred_at: new Date().toISOString()
        };
        
        try {
            const response = await fetch('/api/v1/beads/auto-file', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(bugReport)
            });
            
            if (response.ok) {
                const result = await response.json();
                console.log(`%c✅ Bug report auto-filed: ${result.bead_id}`, 'color: green; font-weight: bold');
                console.log(`   Assigned to: ${result.assigned_to}`);
                console.log(`   Ask your AI assistant to check for "[auto-filed]" beads`);
            } else {
                console.error('❌ Failed to auto-file bug report:', await response.text());
            }
        } catch (error) {
            console.error('❌ Error filing bug report:', error);
        }
    }
    
    // Make it available globally for manual filing
    window.fileUIBug = autoFileBugReport;
    
})();
