import * as vscode from 'vscode';
import { ChatPanelProvider } from './chatPanel';
import { AgentiCorpClient } from './client';
import { AgentiCorpCompletionProvider } from './completionProvider';

let chatPanelProvider: ChatPanelProvider;
let completionProvider: AgentiCorpCompletionProvider;
let client: AgentiCorpClient;

export function activate(context: vscode.ExtensionContext) {
    console.log('AgentiCorp extension is now active');

    // Initialize AgentiCorp client
    const config = vscode.workspace.getConfiguration('agenticorp');
    client = new AgentiCorpClient(
        config.get('apiEndpoint', 'http://localhost:8080'),
        config.get('apiKey', '')
    );

    // Register chat panel provider
    chatPanelProvider = new ChatPanelProvider(context.extensionUri, client);
    
    context.subscriptions.push(
        vscode.window.registerWebviewViewProvider(
            'agenticorp.chatView',
            chatPanelProvider
        )
    );

    // Register inline completion provider
    completionProvider = new AgentiCorpCompletionProvider(client);
    
    context.subscriptions.push(
        vscode.languages.registerInlineCompletionItemProvider(
            { pattern: '**' },
            completionProvider
        )
    );

    // Register commands
    context.subscriptions.push(
        vscode.commands.registerCommand('agenticorp.openChat', () => {
            vscode.commands.executeCommand('workbench.view.extension.agenticorp');
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('agenticorp.askAboutSelection', async () => {
            const editor = vscode.window.activeTextEditor;
            if (!editor) {
                return;
            }

            const selection = editor.document.getText(editor.selection);
            if (!selection) {
                vscode.window.showWarningMessage('Please select some code first');
                return;
            }

            const question = await vscode.window.showInputBox({
                prompt: 'What would you like to know about this code?',
                placeHolder: 'e.g., What does this do?'
            });

            if (question) {
                chatPanelProvider.sendMessage(question, selection);
            }
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('agenticorp.explainCode', async () => {
            const editor = vscode.window.activeTextEditor;
            if (!editor) {
                return;
            }

            const selection = editor.document.getText(editor.selection);
            if (!selection) {
                vscode.window.showWarningMessage('Please select some code first');
                return;
            }

            const language = editor.document.languageId;
            chatPanelProvider.sendMessage(
                `Explain this ${language} code in detail:`,
                selection
            );
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('agenticorp.generateTests', async () => {
            const editor = vscode.window.activeTextEditor;
            if (!editor) {
                return;
            }

            const selection = editor.document.getText(editor.selection);
            if (!selection) {
                vscode.window.showWarningMessage('Please select some code first');
                return;
            }

            const language = editor.document.languageId;
            chatPanelProvider.sendMessage(
                `Generate comprehensive unit tests for this ${language} code:`,
                selection
            );
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('agenticorp.refactorCode', async () => {
            const editor = vscode.window.activeTextEditor;
            if (!editor) {
                return;
            }

            const selection = editor.document.getText(editor.selection);
            if (!selection) {
                vscode.window.showWarningMessage('Please select some code first');
                return;
            }

            const language = editor.document.languageId;
            chatPanelProvider.sendMessage(
                `Refactor this ${language} code to improve readability and maintainability:`,
                selection
            );
        })
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('agenticorp.fixBug', async () => {
            const editor = vscode.window.activeTextEditor;
            if (!editor) {
                return;
            }

            const selection = editor.document.getText(editor.selection);
            if (!selection) {
                vscode.window.showWarningMessage('Please select some code first');
                return;
            }

            const issue = await vscode.window.showInputBox({
                prompt: 'Describe the bug or issue',
                placeHolder: 'e.g., Function returns undefined instead of array'
            });

            if (issue) {
                const language = editor.document.languageId;
                chatPanelProvider.sendMessage(
                    `Help fix this bug in ${language} code. Issue: ${issue}`,
                    selection
                );
            }
        })
    );

    // Listen for configuration changes
    context.subscriptions.push(
        vscode.workspace.onDidChangeConfiguration(e => {
            if (e.affectsConfiguration('agenticorp')) {
                const config = vscode.workspace.getConfiguration('agenticorp');
                client.updateConfig(
                    config.get('apiEndpoint', 'http://localhost:8080'),
                    config.get('apiKey', '')
                );
            }
        })
    );
}

export function deactivate() {
    console.log('AgentiCorp extension is now deactivated');
}
