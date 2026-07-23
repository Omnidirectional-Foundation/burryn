import * as path from "path";
import {
  workspace,
  window,
  ExtensionContext,
} from "vscode";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  TransportKind,
} from "vscode-languageclient/node";

let client: LanguageClient | undefined;

function findBurBinary(): string {
  const config = workspace.getConfiguration("burryn");
  const configured = config.get<string>("serverPath");
  if (configured) {
    return configured;
  }
  return "bur";
}

export function activate(context: ExtensionContext) {
  const burPath = findBurBinary();

  const serverOptions: ServerOptions = {
    command: burPath,
    args: ["lsp"],
    transport: TransportKind.stdio,
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: "file", language: "burryn" }],
    synchronize: {
      fileEvents: workspace.createFileSystemWatcher("**/*.bur"),
    },
  };

  client = new LanguageClient(
    "burryn",
    "Burryn Language Server",
    serverOptions,
    clientOptions
  );

  client.start();
}

export function deactivate(): Thenable<void> | undefined {
  if (!client) {
    return undefined;
  }
  return client.stop();
}
