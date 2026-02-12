// Conversations View - Agent Chat History

async function renderConversationsView() {
    const container = document.getElementById('conversations-container');
    if (!container) return;

    try {
        const projectId = uiState.project.selectedId
            || ((state.projects && state.projects[0]) ? state.projects[0].id : '');

        if (!projectId) {
            container.innerHTML = renderEmptyState(
                'No project selected',
                'Register a project first to see agent conversations'
            );
            return;
        }

        const result = await apiCall(`/conversations?project_id=${encodeURIComponent(projectId)}`);

        // API returns {project_id, limit, conversations: [...]}
        const conversations = Array.isArray(result) ? result : (result && result.conversations) || [];

        if (conversations.length === 0) {
            container.innerHTML = renderEmptyState(
                'No conversations yet',
                'Agent conversations will appear here as they work on beads'
            );
            return;
        }

        container.innerHTML = `
            <div class="conversations-layout">
                <div class="conversations-list">
                    ${conversations.map(conv => renderConversationListItem(conv)).join('')}
                </div>
                <div class="conversation-detail" id="conversation-detail">
                    <div class="empty-state">
                        <p>Select a conversation to view messages</p>
                    </div>
                </div>
            </div>
        `;

        // Auto-select first conversation (only if session_id is valid)
        if (conversations[0] && conversations[0].session_id) {
            loadConversationDetail(conversations[0].session_id);
        }
    } catch (error) {
        container.innerHTML = `<div class="error">Failed to load conversations: ${error.message}</div>`;
    }
}

function renderConversationListItem(conv) {
    const sessionId = conv.session_id || '';
    const agentName = (conv.metadata && conv.metadata.agent_name) || conv.bead_id || sessionId || 'Unknown';
    const messageCount = conv.messages ? conv.messages.length : 0;
    const lastMessage = conv.updated_at ? new Date(conv.updated_at).toLocaleString() : 'No messages';

    if (!sessionId) {
        return `
        <div class="conversation-item" style="opacity: 0.5;" title="Missing session ID">
            <div class="conversation-header">
                <strong>${escapeHtml(agentName)}</strong>
                <span class="badge">${messageCount} msgs</span>
            </div>
            <div class="conversation-meta">
                <small>${escapeHtml(conv.bead_id || 'No bead')}</small>
                <small class="text-muted">${lastMessage}</small>
            </div>
        </div>
        `;
    }

    return `
        <div class="conversation-item" onclick="loadConversationDetail('${escapeHtml(sessionId)}')"
             data-conversation-id="${escapeHtml(sessionId)}">
            <div class="conversation-header">
                <strong>${escapeHtml(agentName)}</strong>
                <span class="badge">${messageCount} msgs</span>
            </div>
            <div class="conversation-meta">
                <small>${escapeHtml(conv.bead_id || 'No bead')}</small>
                <small class="text-muted">${lastMessage}</small>
            </div>
        </div>
    `;
}

async function loadConversationDetail(conversationId) {
    const detailContainer = document.getElementById('conversation-detail');
    if (!detailContainer) return;
    if (!conversationId || conversationId === 'undefined' || conversationId === 'null') {
        detailContainer.innerHTML = '<div class="empty-state"><p>Invalid conversation ID</p></div>';
        return;
    }

    // Highlight selected conversation
    document.querySelectorAll('.conversation-item').forEach(item => {
        item.classList.toggle('active', item.dataset.conversationId === conversationId);
    });

    try {
        detailContainer.innerHTML = '<div class="loading">Loading messages...</div>';

        const conversation = await apiCall(`/conversations/${encodeURIComponent(conversationId)}`, { suppressToast: true, skipAutoFile: true });

        if (!conversation.messages || conversation.messages.length === 0) {
            detailContainer.innerHTML = renderEmptyState('No messages', 'This conversation has no messages yet');
            return;
        }

        const detailAgentName = (conversation.metadata && conversation.metadata.agent_name)
            || conversation.bead_id || 'Agent Conversation';

        detailContainer.innerHTML = `
            <div class="conversation-header-detail">
                <h3>${escapeHtml(detailAgentName)}</h3>
                <div class="conversation-info">
                    <span class="badge">ID: ${conversation.session_id}</span>
                    ${conversation.bead_id ? `<span class="badge">Bead: ${conversation.bead_id}</span>` : ''}
                    <span class="badge">${conversation.messages.length} messages</span>
                </div>
            </div>
            <div class="messages-container">
                ${conversation.messages.map(msg => renderMessage(msg)).join('')}
            </div>
        `;

        // Scroll to bottom
        const messagesContainer = detailContainer.querySelector('.messages-container');
        if (messagesContainer) {
            messagesContainer.scrollTop = messagesContainer.scrollHeight;
        }
    } catch (error) {
        detailContainer.innerHTML = `<div class="error">Failed to load conversation: ${error.message}</div>`;
    }
}

function renderMessage(message) {
    const roleClass = message.role === 'user' ? 'message-user' :
                     message.role === 'assistant' ? 'message-assistant' : 'message-system';

    const roleLabel = message.role === 'user' ? '👤 User' :
                     message.role === 'assistant' ? '🤖 Assistant' : '⚙️ System';

    const timestamp = message.timestamp ? new Date(message.timestamp).toLocaleString() : '';

    return `
        <div class="message ${roleClass}">
            <div class="message-header">
                <span class="message-role">${roleLabel}</span>
                <span class="message-time">${timestamp}</span>
            </div>
            <div class="message-content">
                ${formatMessageContent(message.content)}
            </div>
        </div>
    `;
}

function formatMessageContent(content) {
    if (typeof content === 'string') {
        // Simple markdown-like formatting
        let formatted = escapeHtml(content);

        // Code blocks
        formatted = formatted.replace(/```([^`]+)```/g, '<pre class="code-block">$1</pre>');

        // Inline code
        formatted = formatted.replace(/`([^`]+)`/g, '<code>$1</code>');

        // Bold
        formatted = formatted.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');

        // Line breaks
        formatted = formatted.replace(/\n/g, '<br>');

        return formatted;
    }

    return '<pre class="code-block">' + escapeHtml(JSON.stringify(content, null, 2)) + '</pre>';
}
