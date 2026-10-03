const vscode = require("vscode");
const fs = require("fs");
const path = require("path");
const cp = require("child_process");
const os = require("os");

const output = vscode.window.createOutputChannel("go-clue");

let client = null;
let rebuildTimer = null;
let installPromptOpen = false;
let extensionContext = null;
let goClueCommand = null;
let languageClientModule = null;
let activeLspCommand = null;
let activeLspVersion = "-";

async function activate(context) {
  extensionContext = context;
  context.subscriptions.push(output);
  output.appendLine(`go-clue extension activated at ${new Date().toISOString()}`);

  const watcher = vscode.workspace.createFileSystemWatcher("**/*.{go,gohtml,tmpl,html}");
  context.subscriptions.push(
    watcher,
    watcher.onDidChange((uri) => scheduleRebuild(uri.fsPath)),
    watcher.onDidCreate((uri) => scheduleRebuild(uri.fsPath)),
    watcher.onDidDelete((uri) => scheduleRebuild(uri.fsPath)),
    vscode.workspace.onDidSaveTextDocument((document) => scheduleRebuild(document.uri.fsPath)),
    vscode.commands.registerCommand("go-clue.rebuildIndex", rebuildCurrentWorkspace),
    vscode.commands.registerCommand("go-clue.showIndexStatus", showIndexStatus),
    vscode.commands.registerCommand("go-clue.toggleEnabled", toggleEnabled),
    vscode.commands.registerCommand("go-clue.toggleAutoIndex", toggleAutoIndex),
    vscode.commands.registerCommand("go-clue.restartLsp", restartLsp),
  );

  await startClient(context);
}

async function deactivate() {
  await stopClient();
  output.dispose();
}

async function startClient(context) {
  const root = workspaceRoot();
  if (!root) {
    output.appendLine("No workspace folder found; go-clue LSP not started");
    return;
  }
  if (!goClueEnabled(root)) {
    output.appendLine("go-clue disabled for this workspace");
    return;
  }

  const command = await ensureGoClue(root, false);
  if (!command) {
    output.appendLine("go-clue CLI is not available; LSP not started");
    return;
  }
  const lspCommand = prepareLspCommand(command);

  await stopClient();
  activeLspCommand = lspCommand;
  activeLspVersion = await commandVersion(lspCommand, root);
  output.appendLine(`Starting go-clue LSP: ${lspCommand} lsp ${root} (${activeLspVersion})`);

  const lsp = loadLanguageClient();
  if (!lsp) return;

  client = new lsp.LanguageClient(
    "go-clue",
    "go-clue",
    {
      command: lspCommand,
      args: ["lsp", root],
      options: { cwd: root },
    },
    {
      documentSelector: [
        { language: "go-template", scheme: "file" },
        { language: "go-html-template", scheme: "file" },
        { language: "gotmpl", scheme: "file" },
        { language: "html", scheme: "file" },
        { scheme: "file", pattern: "**/*.gohtml" },
        { scheme: "file", pattern: "**/*.tmpl" },
      ],
      outputChannel: output,
    },
  );

  context.subscriptions.push(client.onDidChangeState((event) => {
    output.appendLine(`go-clue LSP state: ${stateName(event.oldState)} -> ${stateName(event.newState)}`);
  }));
  context.subscriptions.push(client);
  try {
    await client.start();
    output.appendLine("go-clue LSP started");
    await vscode.commands.executeCommand("editor.action.restartSemanticTokensProvider").then(undefined, () => undefined);
  } catch (err) {
    output.appendLine(`go-clue LSP failed to start: ${err.message}`);
    vscode.window.showWarningMessage(`go-clue LSP failed to start: ${err.message}`);
  }
}

async function stopClient() {
  if (!client) return;
  const current = client;
  client = null;
  activeLspCommand = null;
  activeLspVersion = "-";
  try {
    await current.stop();
  } catch (err) {
    output.appendLine(`go-clue LSP stop failed: ${err.message}`);
  }
}

function loadLanguageClient() {
  if (languageClientModule) return languageClientModule;
  try {
    languageClientModule = require("vscode-languageclient/node");
    output.appendLine("vscode-languageclient loaded");
    return languageClientModule;
  } catch (err) {
    const message = `go-clue extension could not load vscode-languageclient: ${err.message}`;
    output.appendLine(message);
    vscode.window.showErrorMessage(message);
    return null;
  }
}

function workspaceRoot() {
  const folder = vscode.workspace.workspaceFolders && vscode.workspace.workspaceFolders[0];
  if (!folder) return null;
  return findModuleRoot(folder.uri.fsPath) || folder.uri.fsPath;
}

function scheduleRebuild(filePath) {
  if (ignoredPath(filePath)) return;
  const root = findModuleRoot(filePath);
  if (!root) return;
  if (!autoIndexEnabled(root)) return;
  clearTimeout(rebuildTimer);
  const delay = vscode.workspace.getConfiguration("go-clue").get("debounceMilliseconds", 1200);
  rebuildTimer = setTimeout(() => rebuildIndex(root, false), delay);
}

function autoIndexEnabled(root) {
  if (vscode.workspace.getConfiguration("go-clue").get("autoIndex", false)) return true;
  const config = projectConfig(root);
  return config && config.writeIndex === true;
}

function goClueEnabled(root) {
  if (!vscode.workspace.getConfiguration("go-clue").get("enabled", true)) return false;
  const config = projectConfig(root);
  return !config || config.enabled !== false;
}

function projectConfig(root) {
  try {
    const configPath = path.join(root, ".go-clue", "config.json");
    if (!fs.existsSync(configPath)) return null;
    const config = JSON.parse(fs.readFileSync(configPath, "utf8"));
    return config || null;
  } catch (err) {
    output.appendLine(`go-clue config read failed: ${err.message}`);
    return null;
  }
}

async function rebuildCurrentWorkspace() {
  const root = workspaceRoot();
  if (!root) return;
  await rebuildIndex(root, true);
}

async function rebuildIndex(root, notify) {
  const command = await ensureGoClue(root, notify);
  if (!command) return;

  const outDir = path.join(root, ".go-clue");
  const outFile = path.join(outDir, "index.json");
  fs.mkdirSync(outDir, { recursive: true });

  const result = await execFile(command, ["index", "-o", outFile, "."], root);
  if (result.err) {
    const message = result.stderr || result.stdout || result.err.message;
    output.appendLine(message);
    vscode.window.showWarningMessage(`go-clue index failed: ${message.slice(0, 160)}`);
    return;
  }

  if (!fs.existsSync(outFile)) {
    const message = (result.stderr || result.stdout || "no template contracts found; index not written").trim();
    if (message) output.appendLine(message);
    if (notify) vscode.window.showInformationMessage("go-clue index not needed: no template contracts found");
  } else if (notify) {
    vscode.window.showInformationMessage("go-clue index rebuilt");
  }

  if (client) {
    await client.stop();
    client = null;
  }
  await startClient(extensionContext || { subscriptions: [] });
}

async function ensureGoClue(root, notify) {
  if (goClueCommand) return goClueCommand;

  const resolved = await resolveGoClue(root);
  if (resolved) {
    goClueCommand = resolved;
    output.appendLine(`go-clue probe succeeded: ${resolved}`);
    return resolved;
  }

  output.appendLine("go-clue probe failed: command not found");
  const installed = await offerInstall(root, notify);
  if (!installed) return null;

  goClueCommand = await resolveGoClue(root);
  return goClueCommand;
}

async function offerInstall(root, notify) {
  if (installPromptOpen) return false;
  installPromptOpen = true;
  try {
    const answer = await vscode.window.showWarningMessage(
      "go-clue is not available on PATH. Install it now with `go install github.com/donseba/go-clue@latest`?",
      { modal: true },
      "Install",
    );
    if (answer !== "Install") return false;

    output.appendLine("Installing go-clue CLI: go install github.com/donseba/go-clue@latest");
    const result = await execFile("go", ["install", "github.com/donseba/go-clue@latest"], root);
    if (result.stdout) output.appendLine(result.stdout);
    if (result.stderr) output.appendLine(result.stderr);
    if (result.err) {
      const message = result.err.code === "ENOENT"
        ? "Go is not available on PATH. Install Go or add it to PATH before installing go-clue."
        : result.stderr || result.stdout || result.err.message;
      vscode.window.showErrorMessage(`go-clue install failed: ${message.slice(0, 180)}`);
      return false;
    }
    if (notify) vscode.window.showInformationMessage("go-clue CLI installed");
    return true;
  } finally {
    installPromptOpen = false;
  }
}

async function resolveGoClue(root) {
  const envCandidates = await goClueCandidatesFromGoEnv(root);
  const pathCandidate = await firstGoCluePath(root);
  const candidates = [...envCandidates, pathCandidate, "go-clue"];

  for (const candidate of unique(candidates.filter(Boolean))) {
    const probe = await execFile(candidate, ["--help"], root);
    if (!probe.err || probe.err.code !== "ENOENT") return candidate;
  }

  return null;
}

async function goClueCandidatesFromGoEnv(root) {
  const result = await execFile("go", ["env", "GOBIN", "GOPATH"], root);
  if (result.err) {
    return [defaultGoClueBin()];
  }

  const lines = result.stdout.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
  const gobin = lines[0] || "";
  const gopath = lines[1] || "";
  const exe = process.platform === "win32" ? "go-clue.exe" : "go-clue";
  const paths = [];
  if (gobin) paths.push(path.join(gobin, exe));
  if (gopath) paths.push(path.join(gopath.split(path.delimiter)[0], "bin", exe));
  paths.push(defaultGoClueBin());
  return paths;
}

function defaultGoClueBin() {
  const exe = process.platform === "win32" ? "go-clue.exe" : "go-clue";
  return path.join(os.homedir(), "go", "bin", exe);
}

function prepareLspCommand(command) {
  if (process.platform !== "win32" || !path.isAbsolute(command) || !fs.existsSync(command)) {
    return command;
  }

  try {
    const dir = extensionContext?.globalStorageUri?.fsPath || path.join(os.tmpdir(), "go-clue-vscode");
    fs.mkdirSync(dir, { recursive: true });
    cleanupOldLspCopies(dir);
    const copy = path.join(dir, `go-clue-lsp-${process.pid}-${Date.now()}.exe`);
    fs.copyFileSync(command, copy);
    output.appendLine(`Copied go-clue LSP binary to ${copy}`);
    return copy;
  } catch (err) {
    output.appendLine(`go-clue LSP binary copy failed, using installed binary: ${err.message}`);
    return command;
  }
}

function cleanupOldLspCopies(dir) {
  try {
    for (const entry of fs.readdirSync(dir)) {
      if (/^go-clue-lsp-\d+-\d+\.exe$/.test(entry)) {
        fs.rmSync(path.join(dir, entry), { force: true });
      }
    }
  } catch (err) {
    output.appendLine(`go-clue LSP copy cleanup skipped: ${err.message}`);
  }
}

function unique(values) {
  return [...new Set(values)];
}

function execFile(command, args, cwd) {
  return new Promise((resolve) => {
    cp.execFile(command, args, { cwd }, (err, stdout, stderr) => {
      resolve({ err, stdout, stderr });
    });
  });
}

function findModuleRoot(filePath) {
  let dir = fs.existsSync(filePath) && fs.statSync(filePath).isDirectory() ? filePath : path.dirname(filePath);
  while (dir && dir !== path.dirname(dir)) {
    if (fs.existsSync(path.join(dir, "go.mod"))) return dir;
    dir = path.dirname(dir);
  }
  return null;
}

function ignoredPath(filePath) {
  const parts = filePath.replaceAll("\\", "/").split("/");
  return parts.some((part) => [".git", ".idea", ".go-clue", "build", "out", "vendor", "node_modules"].includes(part));
}

async function showIndexStatus() {
  const root = workspaceRoot();
  const editor = vscode.window.activeTextEditor;
  const filePath = editor && editor.document.uri.fsPath;
  const languageId = editor && editor.document.languageId;
  const indexFile = root ? path.join(root, ".go-clue", "index.json") : null;
  const exists = indexFile && fs.existsSync(indexFile);
  const goCluePath = root ? (goClueCommand || await resolveGoClue(root) || "-") : "-";
  const installedVersion = root && goCluePath !== "-" ? await commandVersion(goCluePath, root) : "-";
  let templates = 0;
  let types = 0;
  let error = "-";

  if (exists) {
    try {
      const index = JSON.parse(fs.readFileSync(indexFile, "utf8"));
      templates = Object.keys(index.templates || {}).length;
      types = Object.keys(index.types || {}).length;
    } catch (err) {
      error = `${err.name}: ${err.message}`;
    }
  }

  vscode.window.showInformationMessage(
    [
      `Optional index: ${exists ? indexFile : "no optional .go-clue/index.json file"}`,
      `Index root: ${root || "-"}`,
      `Project: ${vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || "-"}`,
      `File: ${filePath || "-"}`,
      `Language: ${languageId || "-"}`,
      `Client: ${client ? stateName(client.state) : "not created"}`,
      `go-clue: ${goCluePath}`,
      `Installed version: ${installedVersion}`,
      `LSP executable: ${activeLspCommand || "-"}`,
      `LSP version: ${activeLspVersion}`,
      `Templates: ${templates}`,
      `Types: ${types}`,
      `Error: ${error}`,
    ].join("\n"),
    { modal: true },
  );
}

async function toggleAutoIndex() {
  const config = vscode.workspace.getConfiguration("go-clue");
  const next = !config.get("autoIndex", false);
  await config.update("autoIndex", next, vscode.ConfigurationTarget.Workspace);
  vscode.window.showInformationMessage(`go-clue auto index ${next ? "enabled" : "disabled"}`);
}

async function toggleEnabled() {
  const config = vscode.workspace.getConfiguration("go-clue");
  const next = !config.get("enabled", true);
  await config.update("enabled", next, vscode.ConfigurationTarget.Workspace);
  vscode.window.showInformationMessage(`go-clue ${next ? "enabled" : "disabled"} for this workspace`);
  if (next) {
    await startClient(extensionContext || { subscriptions: [] });
    return;
  }
  await stopClient();
}

async function restartLsp() {
  output.show(true);
  await startClient(extensionContext || { subscriptions: [] });
  vscode.window.showInformationMessage("go-clue LSP restarted");
}

function stateName(state) {
  const State = languageClientModule && languageClientModule.State;
  if (!State) return String(state);
  switch (state) {
    case State.Stopped:
      return "stopped";
    case State.Starting:
      return "starting";
    case State.Running:
      return "running";
    default:
      return String(state);
  }
}

async function findGoCluePath(root) {
  const command = process.platform === "win32" ? "where" : "which";
  const result = await execFile(command, ["go-clue"], root);
  return (result.stdout || result.stderr || result.err?.message || "-").trim();
}

async function firstGoCluePath(root) {
  const output = await findGoCluePath(root);
  const first = output.split(/\r?\n/).map((line) => line.trim()).find(Boolean);
  return first && first !== "-" ? first : null;
}

async function commandVersion(command, root) {
  const result = await execFile(command, ["version"], root);
  if (result.err) return "-";
  return (result.stdout || result.stderr || "-").trim() || "-";
}

module.exports = { activate, deactivate };
