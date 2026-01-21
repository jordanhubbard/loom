import * as vscode from 'vscode';
import { AgentiCorpClient } from './client';

export class AgentiCorpCompletionProvider implements vscode.InlineCompletionItemProvider {
    private client: AgentiCorpClient;
    private cache: Map<string, { completion: string; timestamp: number }> = new Map();
    private cacheTTL = 60000; // 1 minute

    constructor(client: AgentiCorpClient) {
        this.client = client;
    }

    async provideInlineCompletionItems(
        document: vscode.TextDocument,
        position: vscode.Position,
        context: vscode.InlineCompletionContext,
        token: vscode.CancellationToken
    ): Promise<vscode.InlineCompletionItem[] | undefined> {
        // Don't suggest during selection or if triggered manually
        if (context.selectedCompletionInfo) {
            return undefined;
        }

        const config = vscode.workspace.getConfiguration('agenticorp');
        const inlineSuggestionsEnabled = config.get('inlineSuggestions', true);
        
        if (!inlineSuggestions Enabled) {
            return undefined;
        }

        try {
            // Get context around cursor
            const linePrefix = document.lineAt(position).text.substring(0, position.character);
            const context = this.getDocumentContext(document, position);
            
            // Check cache
            const cacheKey = `${document.fileName}:${position.line}:${linePrefix}`;
            const cached = this.cache.get(cacheKey);
            if (cached && Date.now() - cached.timestamp < this.cacheTTL) {
                return [new vscode.InlineCompletionItem(cached.completion)];
            }

            // Get completion from AgentiCorp
            const completion = await this.getCompletion(context, linePrefix, document.languageId, token);
            
            if (!completion || token.isCancellationRequested) {
                return undefined;
            }

            // Cache result
            this.cache.set(cacheKey, {
                completion,
                timestamp: Date.now()
            });

            return [new vscode.InlineCompletionItem(completion)];
        } catch (error) {
            console.error('AgentiCorp inline completion error:', error);
            return undefined;
        }
    }

    private getDocumentContext(document: vscode.TextDocument, position: vscode.Position): string {
        const config = vscode.workspace.getConfiguration('agenticorp');
        const maxLines = config.get('maxContextLines', 50);
        
        const startLine = Math.max(0, position.line - maxLines);
        const endLine = Math.min(document.lineCount - 1, position.line + Math.floor(maxLines / 2));
        
        const range = new vscode.Range(startLine, 0, endLine, document.lineAt(endLine).text.length);
        return document.getText(range);
    }

    private async getCompletion(
        context: string,
        linePrefix: string,
        language: string,
        token: vscode.CancellationToken
    ): Promise<string | undefined> {
        const prompt = `Complete the following ${language} code. Provide only the completion, no explanations.

Context:
\`\`\`${language}
${context}
\`\`\`

Current line prefix: ${linePrefix}

Completion:`;

        try {
            const response = await this.client.sendMessage([
                {
                    role: 'system',
                    content: 'You are a code completion assistant. Provide only code completions, no explanations.'
                },
                {
                    role: 'user',
                    content: prompt
                }
            ]);

            if (response.choices && response.choices.length > 0) {
                const completion = response.choices[0].message.content;
                
                // Clean up completion
                return this.cleanCompletion(completion, linePrefix);
            }
        } catch (error) {
            console.error('Completion request failed:', error);
        }

        return undefined;
    }

    private cleanCompletion(completion: string, linePrefix: string): string {
        // Remove code fence markers
        let cleaned = completion.replace(/```[a-z]*\n?/g, '').trim();
        
        // Remove explanation text before code
        const lines = cleaned.split('\n');
        let startIdx = 0;
        for (let i = 0; i < lines.length; i++) {
            if (lines[i].trim() && !lines[i].includes('//') && !lines[i].includes('/*')) {
                startIdx = i;
                break;
            }
        }
        
        cleaned = lines.slice(startIdx).join('\n');
        
        // Limit to reasonable length (single line or short block)
        const maxLines = 5;
        const completionLines = cleaned.split('\n').slice(0, maxLines);
        
        return completionLines.join('\n');
    }

    clearCache() {
        this.cache.clear();
    }
}
