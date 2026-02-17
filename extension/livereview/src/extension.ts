import * as path from 'path';
import * as vscode from 'vscode';
import * as util from 'util';
import * as fs from 'fs';
import * as os from 'os';
import { execFile } from 'child_process';
import { ensureLatestExtension, ensureLatestLrc } from './update';

const execFileAsync = util.promisify(execFile);
const DEFAULT_API_URL = 'https://livereview.hexmos.com';
let cachedLrcPath: string | undefined;

type ShellType = 'powershell' | 'cmd' | 'bash';

const detectShellType = (): ShellType => {
	const shell = (vscode.env.shell ?? '').toLowerCase();
	if (/powershell|pwsh/.test(shell)) {
		return 'powershell';
	}
	if (/cmd\.exe/.test(shell)) {
		return 'cmd';
	}
	return 'bash';
};

type APIState = 'uninitialized' | 'initialized';

interface GitExtension {
	readonly enabled: boolean;
	readonly onDidChangeEnablement: vscode.Event<boolean>;
	getAPI(version: 1): API;
}

interface API {
	readonly state: APIState;
	readonly repositories: Repository[];
	readonly onDidOpenRepository: vscode.Event<Repository>;
	readonly onDidCloseRepository: vscode.Event<Repository>;
	readonly onDidChangeState: vscode.Event<APIState>;
	registerPostCommitCommandsProvider?(provider: PostCommitCommandsProvider): vscode.Disposable;
}

interface PostCommitCommandsProvider {
	provideCommands(): Command[];
}

interface Command {
	command: string;
	title: string;
	tooltip?: string;
	arguments?: unknown[];
}

interface Repository {
	readonly rootUri: vscode.Uri;
	readonly state: RepositoryState;
	readonly onDidCommit: vscode.Event<void>;
	readonly onDidCheckout: vscode.Event<void>;
	log(options?: LogOptions): Promise<Commit[]>;
}

interface RepositoryState {
	readonly HEAD: Branch | undefined;
	readonly indexChanges: Change[];
	readonly workingTreeChanges: Change[];
	readonly mergeChanges: Change[];
	readonly onDidChange: vscode.Event<void>;
}

interface Branch {
	readonly name?: string;
	readonly commit?: string;
}

interface Change {
	readonly uri: vscode.Uri;
	readonly renameUri?: vscode.Uri;
	readonly status: Status;
}

interface LogOptions {
	readonly maxEntries?: number;
	readonly path?: string;
	readonly ref?: string;
}

interface Commit {
	readonly hash: string;
	readonly message: string;
}

// Matches VS Code Git extension status enum; only used for labels.
const enum Status {
	INDEX_MODIFIED,
	INDEX_ADDED,
	INDEX_DELETED,
	INDEX_RENAMED,
	INDEX_COPIED,
	MODIFIED,
	DELETED,
	UNTRACKED,
	IGNORED,
	INTENT_TO_ADD,
	ADDED_BY_US,
	ADDED_BY_THEM,
	DELETED_BY_US,
	DELETED_BY_THEM,
	BOTH_ADDED,
	BOTH_DELETED,
	BOTH_MODIFIED
}

const statusLabels: Record<number, string> = {
	[Status.INDEX_MODIFIED]: 'staged edit',
	[Status.INDEX_ADDED]: 'staged add',
	[Status.INDEX_DELETED]: 'staged delete',
	[Status.INDEX_RENAMED]: 'staged rename',
	[Status.INDEX_COPIED]: 'staged copy',
	[Status.MODIFIED]: 'modified',
	[Status.DELETED]: 'deleted',
	[Status.UNTRACKED]: 'untracked',
	[Status.IGNORED]: 'ignored',
	[Status.INTENT_TO_ADD]: 'intent-to-add',
	[Status.ADDED_BY_US]: 'added by us',
	[Status.ADDED_BY_THEM]: 'added by them',
	[Status.DELETED_BY_US]: 'deleted by us',
	[Status.DELETED_BY_THEM]: 'deleted by them',
	[Status.BOTH_ADDED]: 'both added',
	[Status.BOTH_DELETED]: 'both deleted',
	[Status.BOTH_MODIFIED]: 'both modified'
};

export async function activate(context: vscode.ExtensionContext) {
	const output = vscode.window.createOutputChannel('LiveReview Git Hooks');
	const repoSubscriptions = new Map<string, vscode.Disposable[]>();
	let currentApi: API | undefined;
	const lrcConfigPath = path.join(os.homedir(), '.lrc.toml');
	const LAST_VERSION_KEY = 'livereview.lastVersion';
	const extensionVersion = context.extension.packageJSON.version as string;
	const previousVersion = context.globalState.get<string>(LAST_VERSION_KEY);
	const extensionUpdated = Boolean(previousVersion && previousVersion !== extensionVersion);

	context.subscriptions.push(output);

	const logInfo = (message: string) => {
		output.appendLine(message);
	};

	const describeChange = (change: Change): string => {
		const targetName = path.basename(change.uri.fsPath);
		const renameNote = change.renameUri ? ` (from ${path.basename(change.renameUri.fsPath)})` : '';
		return `${statusLabels[change.status] ?? 'change'}: ${targetName}${renameNote}`;
	};

	const readLrcConfig = async (): Promise<{ api_url?: string; api_key?: string } | undefined> => {
		try {
			const content = await fs.promises.readFile(lrcConfigPath, 'utf8');
			const lines = content.split(/\r?\n/);
			const result: { api_url?: string; api_key?: string } = {};
			for (const line of lines) {
				const trimmed = line.trim();
				if (!trimmed || trimmed.startsWith('#')) {
					continue;
				}
				const [key, rawValue] = trimmed.split('=').map(part => part?.trim());
				if (!key || rawValue === undefined) {
					continue;
				}
				const unquoted = rawValue.replace(/^"|"$/g, '');
				if (key === 'api_url') {
					result.api_url = unquoted;
				} else if (key === 'api_key') {
					result.api_key = unquoted;
				}
			}
			return result;
		} catch {
			return undefined;
		}
	};

	const writeLrcConfig = async (apiUrl: string, apiKey: string) => {
		const body = [`api_key = "${apiKey}"`, `api_url = "${apiUrl}"`].join('\n') + '\n';
		await fs.promises.writeFile(lrcConfigPath, body, { encoding: 'utf8' });
	};

	const syncSettingsFromFile = async () => {
		const cfg = vscode.workspace.getConfiguration('livereview');
		const existing = await readLrcConfig();

		if (!existing) {
			await writeLrcConfig(cfg.get<string>('apiUrl', DEFAULT_API_URL), cfg.get<string>('apiKey', ''));
			return;
		}

		const fileApiUrl = existing.api_url ?? DEFAULT_API_URL;
		const fileApiKey = existing.api_key ?? '';

		if (cfg.get<string>('apiUrl') !== fileApiUrl) {
			await cfg.update('apiUrl', fileApiUrl, vscode.ConfigurationTarget.Global);
		}
		if (cfg.get<string>('apiKey') !== fileApiKey) {
			await cfg.update('apiKey', fileApiKey, vscode.ConfigurationTarget.Global);
		}
	};

	const syncFileFromSettings = async () => {
		const cfg = vscode.workspace.getConfiguration('livereview');
		const apiUrl = cfg.get<string>('apiUrl', DEFAULT_API_URL) || DEFAULT_API_URL;
		const apiKey = cfg.get<string>('apiKey', '') || '';
		await writeLrcConfig(apiUrl, apiKey);
	};

	const resolveLrcPath = async (): Promise<string> => {
		if (cachedLrcPath) {
			return cachedLrcPath;
		}
		const envPath = process.env.LRC_BIN?.trim();
		if (envPath) {
			try {
				await fs.promises.access(envPath, fs.constants.X_OK);
				cachedLrcPath = envPath;
				return envPath;
			} catch {
				// ignore invalid env override and continue resolution
			}
		}
		const isWindows = process.platform === 'win32';
		const finder = isWindows ? 'where' : 'which';
		const targets = isWindows ? ['lrc.exe', 'git-lrc.exe'] : ['lrc'];
		for (const target of targets) {
			try {
				const { stdout } = await execFileAsync(finder, [target], { windowsHide: true });
				const candidates = stdout
					.split(/\r?\n/)
					.map(line => line.trim())
					.filter(Boolean);
				if (candidates.length) {
					cachedLrcPath = candidates[0];
					return cachedLrcPath;
				}
			} catch {
				// ignore
			}
		}

		if (isWindows) {
			const candidates = [
				path.join(process.env.ProgramFiles ?? 'C:/Program Files', 'lrc', 'lrc.exe'),
				path.join(process.env.LOCALAPPDATA ?? '', 'Programs', 'lrc', 'lrc.exe'),
				'C:/Program Files/LiveReview/lrc.exe'
			].filter(Boolean);
			const found = candidates.find(fs.existsSync);
			if (found) {
				cachedLrcPath = found;
				return cachedLrcPath;
			}
			throw new Error(
				'Could not find lrc.exe. Install it via the LiveReview installer or set the LRC_BIN environment variable to the full path of lrc.exe.'
			);
		}

		cachedLrcPath = '/usr/local/bin/lrc';
		return cachedLrcPath;
	};

	const extractErrorMessage = (error: unknown): string => {
		if (error instanceof Error) {
			const stderr = (error as { stderr?: string }).stderr;
			const msg = stderr?.trim();
			return msg && msg.length > 0 ? msg : error.message;
		}
		if (typeof error === 'string') {
			return error;
		}
		return String(error);
	};

	const runGit = async (repoPath: string, args: string[]) => {
		return execFileAsync('git', args, { cwd: repoPath, windowsHide: true });
	};

	const runLrcCli = async (args: string[], cwd?: string): Promise<{ stdout: string; stderr: string }> => {
		const lrcPath = await resolveLrcPath();
		try {
			const { stdout, stderr } = await execFileAsync(lrcPath, args, { cwd, windowsHide: true });
			return { stdout: stdout.trim(), stderr: stderr.trim() };
		} catch (error: unknown) {
			throw new Error(extractErrorMessage(error));
		}
	};

	const runPreCommitChecks = async (repoPath: string) => {
		let against = 'HEAD';
		try {
			await runGit(repoPath, ['rev-parse', '--verify', 'HEAD']);
		} catch {
			const { stdout } = await runGit(repoPath, ['hash-object', '-t', 'tree', '/dev/null']);
			against = stdout.trim();
		}

		let allowNonAscii = false;
		try {
			const { stdout } = await runGit(repoPath, ['config', '--type=bool', 'hooks.allownonascii']);
			allowNonAscii = stdout.trim() === 'true';
		} catch {
			// default remains false
		}

		if (!allowNonAscii) {
			const { stdout } = await runGit(repoPath, ['diff', '--cached', '--name-only', '--diff-filter=A', '-z', against]);
			const files = stdout.split('\0').filter(Boolean);
			const bad = files.filter(f => /[^\u0000-\u007f]/.test(f));
			if (bad.length) {
				const err = new Error(`Non-ASCII filename(s) staged: ${bad.join(', ')}`);
				throw err;
			}
		}

		try {
			await runGit(repoPath, ['diff-index', '--check', '--cached', against, '--']);
		} catch (error: unknown) {
			const stderr = (error as { stderr?: string }).stderr ?? '';
			const msg = stderr.trim() || String(error);
			throw new Error(`Whitespace check failed: ${msg}`);
		}
	};

	const readCommitMsgNote = async (repoPath: string, cleanup: boolean): Promise<string | undefined> => {
		const stateFile = path.join(repoPath, '.git', 'livereview_state');
		const lockDir = path.join(repoPath, '.git', 'livereview_state.lock');

		let state: string | undefined;
		try {
			const content = await fs.promises.readFile(stateFile, 'utf8');
			state = content.split(':')[0]?.trim();
		} catch {
			// no state; nothing to report
		}

		let note: string | undefined;
		if (state === 'ran') {
			note = 'LiveReview Pre-Commit Check: ran';
		} else if (state === 'skipped_manual') {
			note = 'LiveReview Pre-Commit Check: skipped manually';
		} else if (state === 'skipped') {
			note = 'LiveReview Pre-Commit Check: skipped';
		}

		if (cleanup) {
			try { await fs.promises.rm(stateFile); } catch {}
			try { await fs.promises.rmdir(lockDir); } catch {}
		}

		return note;
	};

	const runLrc = async (repoPath: string, mode: 'review' | 'skip'): Promise<string> => {
		const termName = `LiveReview lrc (${path.basename(repoPath)})`;
		const term = vscode.window.createTerminal({ name: termName, cwd: repoPath });
		const lrcPath = await resolveLrcPath();

		const args = mode === 'skip'
			? ['review', '--skip']
			: ['review'];

		const shellType = detectShellType();
		let cdPrefix: string;
		let lrcInvoke: string;
		if (shellType === 'powershell') {
			cdPrefix = `Set-Location -LiteralPath "${repoPath}"; `;
			lrcInvoke = `& "${lrcPath}"`;
		} else if (shellType === 'cmd') {
			cdPrefix = `cd /d "${repoPath}" && `;
			lrcInvoke = `"${lrcPath}"`;
		} else {
			cdPrefix = `cd "${repoPath}" && `;
			lrcInvoke = `"${lrcPath}"`;
		}
		const cmd = [lrcInvoke, ...args].join(' ');

		term.show(true);
		term.sendText(cdPrefix + cmd, true);

		return `lrc launched in terminal "${termName}" (${mode}). Check terminal for details.`;
	};

	const runReview = async (repo: Repository, staged: Change[]) => {
		const repoPath = repo.rootUri.fsPath;
		const repoName = path.basename(repoPath);
		const stagedCount = staged.length;

		await vscode.window.withProgress({ location: vscode.ProgressLocation.Notification, title: `LiveReview checks for ${repoName}` }, async (progress) => {
			try {
				progress.report({ message: 'Running pre-commit checks...' });
				// await runPreCommitChecks(repoPath);

				progress.report({ message: 'Launching lrc (interactive terminal)...' });
				const launchNote = await runLrc(repoPath, 'review');

				const summaryLines = [
					`🚀 LiveReview started for ${stagedCount} staged file(s).`,
					`🔧 ${launchNote}`,
					'⏳ Review is running in the terminal; watch for completion there.'
				];

				const summary = summaryLines.join('\n');
				logInfo(`[review] ${repoName}:\n${summary}`);
				vscode.window.showInformationMessage(summary, 'Open Output').then(sel => {
					if (sel === 'Open Output') {
						output.show(true);
					}
				});
			} catch (error: unknown) {
				const message = (error as Error)?.message ?? String(error);
				logInfo(`[review error] ${repoName}: ${message}`);
				vscode.window.showErrorMessage(`LiveReview checks failed: ${message}`, 'Open Output').then(sel => {
					if (sel === 'Open Output') {
						output.show(true);
					}
				});
			}
		});
	};

	const runSkipReview = async (repo: Repository, staged: Change[]) => {
		const repoPath = repo.rootUri.fsPath;
		const repoName = path.basename(repoPath);
		const stagedCount = staged.length;

		await vscode.window.withProgress({ location: vscode.ProgressLocation.Notification, title: `LiveReview skip for ${repoName}` }, async (progress) => {
			try {
				progress.report({ message: 'Launching lrc skip (interactive terminal)...' });
				const launchNote = await runLrc(repoPath, 'skip');

				const summaryLines = [
					`⏭️ LiveReview skipped for ${stagedCount} staged file(s).`,
					`🔧 ${launchNote}`
				];

				const summary = summaryLines.join('\n');
				logInfo(`[review skip] ${repoName}:\n${summary}`);
				vscode.window.showInformationMessage(summary, 'Open Output').then(sel => {
					if (sel === 'Open Output') {
						output.show(true);
					}
				});
			} catch (error: unknown) {
				const message = (error as Error)?.message ?? String(error);
				logInfo(`[review skip error] ${repoName}: ${message}`);
				vscode.window.showErrorMessage(`LiveReview skip failed: ${message}`, 'Open Output').then(sel => {
					if (sel === 'Open Output') {
						output.show(true);
					}
				});
			}
		});
	};

	const runReviewForActiveRepo = async (explicitRepo?: Repository) => {
		const repo = explicitRepo ?? await pickRepo();
		if (!repo) {
			return;
		}

		const staged = repo.state.indexChanges;
		if (!staged.length) {
			vscode.window.showWarningMessage('LiveReview: No staged files to review.');
			return;
		}

		void runReview(repo, staged);
	};

	const runSkipForActiveRepo = async (explicitRepo?: Repository) => {
		const repo = explicitRepo ?? await pickRepo();
		if (!repo) {
			return;
		}

		const staged = repo.state.indexChanges;
		if (!staged.length) {
			vscode.window.showWarningMessage('LiveReview: No staged files to skip.');
			return;
		}

		void runSkipReview(repo, staged);
	};

	const runReviewForRepo = async (repo: Repository) => {
		const staged = repo.state.indexChanges;
		if (!staged.length) {
			vscode.window.showWarningMessage('LiveReview: No staged files to review.');
			return;
		}
		void runReview(repo, staged);
	};

	const tryGetRepoFromArg = (arg: unknown): Repository | undefined => {
		const candidate = arg as Partial<Repository> | undefined;
		if (!candidate || !candidate.rootUri) {
			return undefined;
		}
		if (!(candidate.rootUri instanceof vscode.Uri)) {
			return undefined;
		}
		if (candidate.state && Array.isArray(candidate.state.indexChanges)) {
			return candidate as Repository;
		}
		return undefined;
	};

	const findRepoForFsPath = (fsPath: string): Repository | undefined => {
		const repos = currentApi?.repositories ?? [];
		const normalized = path.resolve(fsPath);
		return repos.find(repo => {
			const root = path.resolve(repo.rootUri.fsPath);
			return normalized === root || normalized.startsWith(root + path.sep);
		});
	};

	const resolveRepoFromArgs = (args: unknown[]): Repository | undefined => {
		for (const arg of args) {
			const directRepo = tryGetRepoFromArg(arg);
			if (directRepo) {
				return directRepo;
			}

			if (arg instanceof vscode.Uri) {
				const repo = findRepoForFsPath(arg.fsPath);
				if (repo) {
					return repo;
				}
			}

			const withRoot = arg as { rootUri?: vscode.Uri };
			if (withRoot?.rootUri instanceof vscode.Uri) {
				const repo = findRepoForFsPath(withRoot.rootUri.fsPath);
				if (repo) {
					return repo;
				}
			}

			const withResource = arg as { resourceUri?: vscode.Uri };
			if (withResource?.resourceUri instanceof vscode.Uri) {
				const repo = findRepoForFsPath(withResource.resourceUri.fsPath);
				if (repo) {
					return repo;
				}
			}

			const group = arg as { resourceStates?: Array<{ resourceUri?: vscode.Uri }> };
			if (group?.resourceStates?.length) {
				for (const state of group.resourceStates) {
					if (state?.resourceUri instanceof vscode.Uri) {
						const repo = findRepoForFsPath(state.resourceUri.fsPath);
						if (repo) {
							return repo;
						}
					}
				}
			}

			if (Array.isArray(arg)) {
				const nested = resolveRepoFromArgs(arg);
				if (nested) {
					return nested;
				}
			}

			if (typeof arg === 'string') {
				const repo = findRepoForFsPath(arg);
				if (repo) {
					return repo;
				}
			}
		}
		return undefined;
	};

	const pickRepo = async (): Promise<Repository | undefined> => {
		const repos = currentApi?.repositories ?? [];
		if (!repos.length) {
			vscode.window.showWarningMessage('LiveReview: No Git repository detected.');
			return undefined;
		}
		if (repos.length === 1) {
			return repos[0];
		}
		const picks = repos.map(r => ({
			label: path.basename(r.rootUri.fsPath),
			description: r.rootUri.fsPath,
			repo: r
		}));
		const choice = await vscode.window.showQuickPick(picks, { placeHolder: 'Select a repository for LiveReview hooks' });
		return choice?.repo;
	};

	const runHookAction = async (action: 'installGlobal' | 'uninstallGlobal' | 'enableLocal' | 'disableLocal' | 'status', explicitRepo?: Repository) => {
		const repo = action === 'installGlobal' || action === 'uninstallGlobal' ? undefined : (explicitRepo ?? await pickRepo());
		if (action !== 'installGlobal' && action !== 'uninstallGlobal' && !repo) {
			return;
		}

		const cwd = repo?.rootUri.fsPath;
		const titleMap: Record<typeof action, string> = {
			installGlobal: 'Install global hooks',
			uninstallGlobal: 'Uninstall global hooks',
			enableLocal: 'Enable hooks (repo)',
			disableLocal: 'Disable hooks (repo)',
			status: 'Hook status (repo)'
		};
		const argsMap: Record<typeof action, string[]> = {
			installGlobal: ['hooks', 'install'],
			uninstallGlobal: ['hooks', 'uninstall'],
			enableLocal: ['hooks', 'enable'],
			disableLocal: ['hooks', 'disable'],
			status: ['hooks', 'status']
		};

		const title = titleMap[action];
		await vscode.window.withProgress({ location: vscode.ProgressLocation.Notification, title: `LiveReview: ${title}` }, async (progress) => {
			progress.report({ message: 'Running lrc...' });
			try {
				const { stdout, stderr } = await runLrcCli(argsMap[action], cwd);
				const sections: string[] = [];
				if (stdout) {
					sections.push(`stdout:\n${stdout}`);
				}
				if (stderr) {
					sections.push(`stderr:\n${stderr}`);
				}
				const lines = sections.join('\n---\n') || 'Done.';
				logInfo(`[hooks] ${title}${cwd ? ` @ ${cwd}` : ''}\n${lines}`);
				vscode.window.showInformationMessage(`LiveReview: ${title} succeeded.`, 'Open Output').then(sel => {
					if (sel === 'Open Output') {
						output.show(true);
					}
				});
			} catch (error: unknown) {
				const message = extractErrorMessage(error);
				logInfo(`[hooks error] ${title}${cwd ? ` @ ${cwd}` : ''}: ${message}`);
				vscode.window.showErrorMessage(`LiveReview: ${title} failed: ${message}`, 'Open Output').then(sel => {
					if (sel === 'Open Output') {
						output.show(true);
					}
				});
			}
		});
	};

	const manageHooks = async () => {
		const picks = [
			{ label: 'Install global hooks', action: 'installGlobal' as const },
			{ label: 'Uninstall global hooks', action: 'uninstallGlobal' as const },
			{ label: 'Enable hooks for this repo', action: 'enableLocal' as const },
			{ label: 'Disable hooks for this repo', action: 'disableLocal' as const },
			{ label: 'Show hook status (repo)', action: 'status' as const }
		];
		const choice = await vscode.window.showQuickPick(picks, { placeHolder: 'LiveReview hook actions' });
		if (!choice) {
			return;
		}
		void runHookAction(choice.action);
	};

	const handleStaging = (repo: Repository) => {
		const staged = repo.state.indexChanges;

		if (!staged.length) {
			return;
		}

		const sample = staged.slice(0, 3).map(describeChange);
		const extraCount = staged.length - sample.length;
		const repoName = path.basename(repo.rootUri.fsPath);
		const message = extraCount > 0
			? `🔔 Staged ${staged.length} files in ${repoName}: ${sample.join(', ')} (+${extraCount} more). Run LiveReview?`
			: `🔔 Staged ${staged.length} files in ${repoName}: ${sample.join(', ')}. Run LiveReview?`;

		// Notification disabled — too noisy. Users can invoke review/skip manually.
		logInfo(`[staging] ${message}`);
	};

	const getLatestCommit = async (repo: Repository): Promise<Commit | undefined> => {
		try {
			const commits = await repo.log({ maxEntries: 1 });
			return commits[0];
		} catch (error) {
			logInfo(`Failed to read latest commit: ${String(error)}`);
			return undefined;
		}
	};

	const handleCommit = async (repo: Repository) => {
		const head = repo.state.HEAD;
		const latest = await getLatestCommit(repo);
		const branchName = head?.name ?? 'detached HEAD';
		const hash = latest?.hash ?? head?.commit ?? 'unknown hash';
		const message = latest?.message ?? 'no commit message found';
		const repoName = path.basename(repo.rootUri.fsPath);

		logInfo(`[commit] ${repoName} on ${branchName}: ${hash} — ${message}`);
		// Notification disabled — too noisy.
	};

	const attachRepo = (repo: Repository) => {
		const key = repo.rootUri.fsPath;
		if (repoSubscriptions.has(key)) {
			return;
		}

		logInfo(`Attaching LiveReview hooks to ${key}`);

		const disposables: vscode.Disposable[] = [];

		disposables.push(repo.state.onDidChange(() => handleStaging(repo)));
		disposables.push(repo.onDidCommit(() => void handleCommit(repo)));

		repoSubscriptions.set(key, disposables);
		context.subscriptions.push(...disposables);

	};

	const detachRepo = (repo: Repository) => {
		const key = repo.rootUri.fsPath;
		const disposables = repoSubscriptions.get(key);
		if (!disposables) {
			return;
		}
		logInfo(`Detaching LiveReview hooks from ${key}`);
		disposables.forEach(d => d.dispose());
		repoSubscriptions.delete(key);
	};

	const wireGitAPI = (api: API) => {
		currentApi = api;
		api.repositories.forEach(attachRepo);
		context.subscriptions.push(api.onDidOpenRepository(repo => attachRepo(repo)));
		context.subscriptions.push(api.onDidCloseRepository(repo => detachRepo(repo)));

		if (api.registerPostCommitCommandsProvider) {
			const provider: PostCommitCommandsProvider = {
				provideCommands: () => [{ command: 'livereview.enableGitHooks', title: 'LiveReview: Post-commit review mock' }]
			};
			context.subscriptions.push(api.registerPostCommitCommandsProvider(provider));
		}
	};

	const initGit = async () => {
		const gitExtension = vscode.extensions.getExtension<GitExtension>('vscode.git');
		if (!gitExtension) {
			vscode.window.showWarningMessage('LiveReview: Git extension not found.');
			return;
		}

		const git = gitExtension.exports;
		if (!git) {
			vscode.window.showWarningMessage('LiveReview: Git extension exports are unavailable.');
			return;
		}

		if (!git.enabled) {
			vscode.window.showWarningMessage('LiveReview: Git extension is disabled. Enable it to use LiveReview hooks.');
			return;
		}

		let api: API;
		try {
			api = git.getAPI(1);
		} catch (error) {
			vscode.window.showWarningMessage(`LiveReview: Unable to acquire Git API: ${String(error)}`);
			return;
		}

		if (api.state === 'initialized') {
			wireGitAPI(api);
		} else {
			const ready = api.onDidChangeState(state => {
				if (state === 'initialized') {
					wireGitAPI(api);
					ready.dispose();
				}
			});
			context.subscriptions.push(ready);
		}
	};

	const enableHooksCommand = vscode.commands.registerCommand('livereview.enableGitHooks', () => {
		vscode.window.showInformationMessage('LiveReview: (Re)initializing Git hooks...');
		void initGit();
	});

	const runLiveReviewCommand = vscode.commands.registerCommand('livereview.runLiveReview', (...args: unknown[]) => {
		const repo = resolveRepoFromArgs(args);
		void runReviewForActiveRepo(repo);
	});

	const skipLiveReviewCommand = vscode.commands.registerCommand('livereview.skipLiveReview', (...args: unknown[]) => {
		const repo = resolveRepoFromArgs(args);
		void runSkipForActiveRepo(repo);
	});

	const manageHooksCommand = vscode.commands.registerCommand('livereview.manageHooks', () => {
		void manageHooks();
	});

	const installGlobalHooksCommand = vscode.commands.registerCommand('livereview.hooks.installGlobal', (...args: unknown[]) => {
		const repo = resolveRepoFromArgs(args);
		void runHookAction('installGlobal', repo);
	});

	const uninstallGlobalHooksCommand = vscode.commands.registerCommand('livereview.hooks.uninstallGlobal', (...args: unknown[]) => {
		const repo = resolveRepoFromArgs(args);
		void runHookAction('uninstallGlobal', repo);
	});

	const enableLocalHooksCommand = vscode.commands.registerCommand('livereview.hooks.enableLocal', (...args: unknown[]) => {
		const repo = resolveRepoFromArgs(args);
		void runHookAction('enableLocal', repo);
	});

	const disableLocalHooksCommand = vscode.commands.registerCommand('livereview.hooks.disableLocal', (...args: unknown[]) => {
		const repo = resolveRepoFromArgs(args);
		void runHookAction('disableLocal', repo);
	});

	const statusHooksCommand = vscode.commands.registerCommand('livereview.hooks.status', (...args: unknown[]) => {
		const repo = resolveRepoFromArgs(args);
		void runHookAction('status', repo);
	});

	context.subscriptions.push(
		enableHooksCommand,
		runLiveReviewCommand,
		skipLiveReviewCommand,
		manageHooksCommand,
		installGlobalHooksCommand,
		uninstallGlobalHooksCommand,
		enableLocalHooksCommand,
		disableLocalHooksCommand,
		statusHooksCommand
	);

	// Update prompts disabled — too noisy.
	// await ensureLatestExtension(context, output).catch(err => {
	// 	output.appendLine(`LiveReview: Extension version check failed: ${String(err)}`);
	// });
	// await ensureLatestLrc(resolveLrcPath, output, { forceRemoteRefresh: extensionUpdated }).catch(err => {
	// 	output.appendLine(`LiveReview: lrc version check failed: ${String(err)}`);
	// });

	void context.globalState.update(LAST_VERSION_KEY, extensionVersion);

	void syncSettingsFromFile().finally(() => {
		void initGit();
	});

	context.subscriptions.push(vscode.workspace.onDidChangeConfiguration(event => {
		if (event.affectsConfiguration('livereview.apiUrl') || event.affectsConfiguration('livereview.apiKey')) {
			void syncFileFromSettings();
		}
	}));
}

export function deactivate() {
	// Nothing to clean up beyond disposables tracked in context.
}
