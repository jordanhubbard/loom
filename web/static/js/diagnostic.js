// Loom UI Error Handler and Auto-Filer
// Catches ALL JS errors and auto-files them as beads

(function() {
    'use strict';
    
    // Track errors to avoid duplicate filings
    const filedErrors = new Set();
    const ERROR_DEBOUNCE_MS = 5000; // Don't file same error within 5 seconds
    const MAX_FILES_PER_MINUTE = 5; // Rate limit: max 5 auto-files per minute
    let fileCountThisMinute = 0;
    setInterval(() => { fileCountThisMinute = 0; }, 60000);
    
    // Show toast notification (uses app.js showToast if available, falls back to console)
    function showErrorToast(message, type = 'error') {
        if (typeof showToast === 'function') {
            showToast(message, type);
        } else {
            console.log(`[Toast ${type}] ${message}`);
        }
    }
    
    // Auto-file error to backend
    async function autoFileError(errorInfo) {
        const errorType = errorInfo.errorType || 'js_error';
        const severity = errorInfo.severity || (errorType === 'api_error' ? 'critical' : 'high');
        const source = errorInfo.source || 'frontend';
        const context = {
            url: window.location.href,
            source_file: errorInfo.source || 'unknown',
            line: errorInfo.lineno || 0,
            column: errorInfo.colno || 0,
            user_agent: navigator.userAgent,
            timestamp: new Date().toISOString(),
            viewport: `${window.innerWidth}x${window.innerHeight}`,
            state: typeof state !== 'undefined' ? {
                beads: state.beads?.length || 0,
                projects: state.projects?.length || 0,
                agents: state.agents?.length || 0,
                providers: state.providers?.length || 0
            } : 'state not available'
        };

        if (errorInfo.context && typeof errorInfo.context === 'object') {
            Object.assign(context, errorInfo.context);
        }

        const endpointKey = context.endpoint || '';
        const statusKey = context.status || '';
        // Create a unique key for deduplication
        const errorKey = `${errorType}:${errorInfo.message}:${source}:${errorInfo.lineno}:${endpointKey}:${statusKey}`;
        
        // Check if we've already filed this error recently
        if (filedErrors.has(errorKey)) {
            return;
        }
        // Rate limit auto-filing
        if (fileCountThisMinute >= MAX_FILES_PER_MINUTE) {
            console.log('[Diagnostic] Rate limit reached, skipping auto-file');
            return;
        }
        filedErrors.add(errorKey);
        fileCountThisMinute++;
        
        // Clear from set after debounce period
        setTimeout(() => filedErrors.delete(errorKey), ERROR_DEBOUNCE_MS);
        
        // Show immediate toast
        showErrorToast('Internal UI error detected', 'error');
        
        const titlePrefix = errorType === 'api_error' ? 'API Error' : 'UI Error';
        const bugReport = {
            title: `${titlePrefix}: ${(errorInfo.message || 'Unknown error').substring(0, 80)}`,
            source: source,
            error_type: errorType,
            message: errorInfo.message || 'Unknown error',
            stack_trace: errorInfo.stack || '',
            context: context,
            severity: severity,
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
                showErrorToast(`Bug filed: ${result.bead_id}`, 'success');
                console.log(`[AutoFile] Bug report filed: ${result.bead_id}, assigned to: ${result.assigned_to}`);
            } else {
                const errorText = await response.text();
                showErrorToast('Failed to file bug report', 'error');
                console.error('[AutoFile] Failed to file bug report:', errorText);
            }
        } catch (error) {
            showErrorToast('Cannot reach backend to file bug', 'error');
            console.error('[AutoFile] Error filing bug report:', error);
        }
    }
    
    // Global error handler for uncaught errors
    window.onerror = function(message, source, lineno, colno, error) {
        console.error('[GlobalError]', message, 'at', source, lineno, colno);
        
        autoFileError({
            message: message,
            source: source,
            lineno: lineno,
            colno: colno,
            stack: error?.stack || ''
        });
        
        // Don't prevent default error handling
        return false;
    };
    
    // Global handler for unhandled promise rejections
    window.addEventListener('unhandledrejection', function(event) {
        const error = event.reason;
        const message = error?.message || String(error) || 'Unhandled promise rejection';
        
        console.error('[UnhandledRejection]', message);
        
        autoFileError({
            message: `Promise rejection: ${message}`,
            source: 'promise',
            lineno: 0,
            colno: 0,
            stack: error?.stack || ''
        });
    });
    
    // Intercept console.error to catch errors logged but not thrown
    const originalConsoleError = console.error;
    console.error = function(...args) {
        // Call original first
        originalConsoleError.apply(console, args);
        
        // Check if this looks like an actual error (not just a log)
        const errorMsg = args.map(a => String(a)).join(' ');
        
        // Filter out expected/handled errors to avoid noise
        const isExpectedError = 
            errorMsg.includes('[GlobalError]') ||       // Already handled by onerror
            errorMsg.includes('[UnhandledRejection]') || // Already handled
            errorMsg.includes('[AutoFile]') ||          // Our own logging
            errorMsg.includes('[Loom] API Error:') ||   // Avoid double-filing API errors
            errorMsg.includes('[Diagnostic]') ||        // Our own init logging
            errorMsg.includes('not found') ||           // 404s are expected for missing resources
            errorMsg.includes('session not found') ||   // Conversation lookups that miss are normal
            errorMsg.includes('Failed to load');        // Transient load failures handled by UI
        
        // File if it contains error indicators and isn't expected
        const looksLikeError = 
            errorMsg.includes('Error') ||
            errorMsg.includes('error') ||
            errorMsg.includes('Cannot read') ||
            errorMsg.includes('undefined') ||
            errorMsg.includes('is not a function') ||
            errorMsg.includes('is not defined');
        
        if (looksLikeError && !isExpectedError) {
            autoFileError({
                message: errorMsg.substring(0, 200),
                source: 'console.error',
                lineno: 0,
                colno: 0,
                stack: ''
            });
        }
    };
    
    // Manual filing function available globally
    window.fileUIBug = function(message, options = {}) {
        autoFileError({
            message: message || 'Manually reported UI issue',
            source: options.source || 'manual',
            lineno: 0,
            colno: 0,
            stack: new Error().stack,
            errorType: options.errorType,
            severity: options.severity,
            context: options.context
        });
    };

    window.fileApiBug = function(details = {}) {
        // Only auto-file server errors (5xx), not client errors (4xx)
        const status = details.status || 0;
        if (status >= 400 && status < 500) {
            return;
        }
        autoFileError({
            message: details.message || `API Error: ${details.method || 'GET'} ${details.endpoint || ''}`.trim(),
            source: 'frontend',
            lineno: 0,
            colno: 0,
            stack: details.stack || '',
            errorType: 'api_error',
            severity: 'critical',
            context: {
                endpoint: details.endpoint || '',
                method: details.method || 'GET',
                status: status,
                response: details.response || ''
            }
        });
    };
    
    console.log('[Diagnostic] UI error auto-filer initialized');
    
})();
