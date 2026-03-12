import * as https from 'https';
import * as util from 'util';
import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import { exec, execFile } from 'child_process';

const execFileAsync = util.promisify(execFile);
const execAsync = util.promisify(exec);

const LRC_RELEASE_MANIFEST_URL = 'https://f005.backblazeb2.com/file/hexmos/lrc/latest.json';

const VERSION_CACHE_TTL_MS = 5 * 60 * 1000;

interface ManifestCacheEntry {
	latestVersion: string;
	expiresAt: number;
}

let manifestCache: ManifestCacheEntry | undefined;

export type LrcUpdateState = 'checking' | 'upToDate' | 'updateAvailable' | 'updating' | 'updated' | 'needsMigration' | 'failed';

export interface LrcUpdateStatus {
	state: LrcUpdateState;
	message: string;
	localVersion?: string;
	remoteVersion?: string;
}

export interface EnsureLatestLrcOptions {
	forceRemoteRefresh?: boolean;
	checkOnly?: boolean;
	backgroundTimeoutMs?: number;
	notifyOnBackgroundSuccess?: boolean;
	onStatus?: (status: LrcUpdateStatus) => void;
}

const semverFromString = (input: string): string | undefined => {
	const match = input.trim().match(/v?(\d+)\.(\d+)\.(\d+)/);
	return match ? `v${match[1]}.${match[2]}.${match[3]}` : undefined;
};

const semverCompare = (a: string, b: string): number => {
	const parse = (v: string) => v.replace(/^v/, '').split('.').map(n => parseInt(n, 10));
	const [a1, a2, a3] = parse(a);
	const [b1, b2, b3] = parse(b);
	if (a1 !== b1) { return a1 > b1 ? 1 : -1; }
	if (a2 !== b2) { return a2 > b2 ? 1 : -1; }
	if (a3 !== b3) { return a3 > b3 ? 1 : -1; }
	return 0;
};

const httpRequest = (options: https.RequestOptions, body?: string): Promise<string> => {
	return new Promise((resolve, reject) => {
		const req = https.request(options, res => {
			const chunks: Buffer[] = [];
			res.on('data', chunk => chunks.push(chunk));
			res.on('end', () => {
				const text = Buffer.concat(chunks).toString('utf8');
				if ((res.statusCode ?? 0) >= 400) {
					reject(new Error(`HTTP ${res.statusCode}: ${text}`));
					return;
				}
				resolve(text);
			});
		});
		req.on('error', reject);
		if (body) {
			req.write(body);
		}
		req.end();
	});
};

const fetchLatestLrcVersionFromManifest = async (forceRefresh = false): Promise<string | undefined> => {
	const now = Date.now();
	if (!forceRefresh && manifestCache && manifestCache.expiresAt > now) {
		return manifestCache.latestVersion;
	}

	const manifestUrl = new URL(LRC_RELEASE_MANIFEST_URL);
	const manifestRequest: https.RequestOptions = {
		method: 'GET',
		hostname: manifestUrl.hostname,
		path: `${manifestUrl.pathname}${manifestUrl.search}`
	};

	const manifestRaw = await httpRequest(manifestRequest);
	const manifest = JSON.parse(manifestRaw) as { latest_version?: string };
	const latest = semverFromString(manifest.latest_version ?? '');
	if (!latest) {
		return undefined;
	}

	manifestCache = { latestVersion: latest, expiresAt: now + VERSION_CACHE_TTL_MS };
	return latest;
};

const getLocalLrcVersion = async (resolveLrcPath: () => Promise<string>): Promise<string | undefined> => {
	try {
		const lrcPath = await resolveLrcPath();
		const { stdout } = await execFileAsync(lrcPath, ['version']);
		return semverFromString(stdout) ?? semverFromString(stdout.split(/\s+/).pop() ?? '');
	} catch {
		return undefined;
	}
};

const platformInstallCommand = (): string => {
	if (process.platform === 'win32') {
		return 'iwr -useb https://hexmos.com/lrc-install.ps1 | iex';
	}
	return 'curl -fsSL https://hexmos.com/lrc-install.sh | bash';
};

const openInstallerTerminal = (title: string, command: string) => {
	const term = vscode.window.createTerminal({ name: title });
	term.show(true);
	term.sendText(command, true);
};

const emitStatus = (
	status: LrcUpdateStatus,
	output: vscode.OutputChannel,
	onStatus?: (status: LrcUpdateStatus) => void
) => {
	onStatus?.(status);
	output.appendLine(`LiveReview: ${status.message}`);
};

const fileExists = async (targetPath: string): Promise<boolean> => {
	try {
		await fs.promises.access(targetPath, fs.constants.F_OK);
		return true;
	} catch {
		return false;
	}
};

const detectGitBinDir = async (): Promise<string | undefined> => {
	try {
		if (process.platform === 'win32') {
			const { stdout } = await execFileAsync('where', ['git'], { windowsHide: true });
			const candidate = stdout
				.split(/\r?\n/)
				.map(line => line.trim())
				.find(line => line.length > 0);
			return candidate ? path.dirname(candidate) : undefined;
		}

		const { stdout } = await execFileAsync('which', ['git']);
		const candidate = stdout
			.split(/\r?\n/)
			.map(line => line.trim())
			.find(line => line.length > 0);
		return candidate ? path.dirname(candidate) : undefined;
	} catch {
		return undefined;
	}
};

const hasLegacyBinaries = async (): Promise<boolean> => {
	if (process.platform === 'win32') {
		const candidates: string[] = [
			path.join(process.env.ProgramFiles ?? 'C:/Program Files', 'lrc', 'lrc.exe')
		];
		const gitBinDir = await detectGitBinDir();
		if (gitBinDir) {
			candidates.push(path.join(gitBinDir, 'git-lrc.exe'));
		}

		for (const candidate of candidates) {
			if (await fileExists(candidate)) {
				return true;
			}
		}
		return false;
	}

	const candidates: string[] = ['/usr/local/bin/lrc', '/usr/local/bin/git-lrc'];
	const gitBinDir = await detectGitBinDir();
	if (gitBinDir && gitBinDir !== '/usr/local/bin') {
		candidates.push(path.join(gitBinDir, 'git-lrc'));
	}

	for (const candidate of candidates) {
		if (await fileExists(candidate)) {
			return true;
		}
	}
	return false;
};

const runBackgroundInstaller = async (timeoutMs: number): Promise<'ok' | 'timeout'> => {
	try {
		if (process.platform === 'win32') {
			await execFileAsync(
				'powershell',
				['-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command', 'iwr -useb https://hexmos.com/lrc-install.ps1 | iex'],
				{ windowsHide: true, timeout: timeoutMs }
			);
			return 'ok';
		}

		await execAsync('curl -fsSL https://hexmos.com/lrc-install.sh | bash', { timeout: timeoutMs });
		return 'ok';
	} catch (error: unknown) {
		const code = (error as { code?: string }).code;
		const killed = Boolean((error as { killed?: boolean }).killed);
		if (code === 'ETIMEDOUT' || killed) {
			return 'timeout';
		}
		throw error;
	}
};

export const ensureLatestLrc = async (
	resolveLrcPath: () => Promise<string>,
	output: vscode.OutputChannel,
	opts?: EnsureLatestLrcOptions
): Promise<LrcUpdateStatus> => {
	const emit = (status: LrcUpdateStatus) => emitStatus(status, output, opts?.onStatus);
	emit({ state: 'checking', message: 'Checking lrc version and update path.' });

	const localVersion = await getLocalLrcVersion(resolveLrcPath);
	const remoteVersion = await fetchLatestLrcVersionFromManifest(opts?.forceRemoteRefresh ?? false);
	const updateNeeded = !localVersion || (remoteVersion ? semverCompare(localVersion, remoteVersion) < 0 : false);
	const legacyMigrationRequired = await hasLegacyBinaries();
	const currentLabel = localVersion ?? 'not installed';

	if (!remoteVersion && !localVersion) {
		if (opts?.checkOnly) {
			const status: LrcUpdateStatus = {
				state: legacyMigrationRequired ? 'needsMigration' : 'updateAvailable',
				message: legacyMigrationRequired
					? 'lrc is not installed and legacy migration is required.'
					: 'lrc is not installed and can be installed rootlessly.'
			};
			emit(status);
			return status;
		}

		if (legacyMigrationRequired) {
			const status: LrcUpdateStatus = {
				state: 'needsMigration',
				message: 'Legacy lrc binaries detected. User confirmation is required for migration.'
			};
			emit(status);
			const choice = await vscode.window.showInformationMessage('LiveReview CLI is not installed. Run migration update now?', { modal: true }, 'Run update', 'Later');
			if (choice === 'Run update') {
				emit({ state: 'updating', message: 'Running installer in terminal for migration.' });
				openInstallerTerminal('LiveReview lrc migration', platformInstallCommand());
			}
			return status;
		}

		emit({ state: 'updating', message: 'Installing lrc in background (rootless).' });
		const backgroundResult = await runBackgroundInstaller(opts?.backgroundTimeoutMs ?? 120000);
		if (backgroundResult === 'timeout') {
			const status: LrcUpdateStatus = {
				state: 'needsMigration',
				message: 'Background install timed out. Migration update may require user confirmation.'
			};
			emit(status);
			const choice = await vscode.window.showInformationMessage('LiveReview CLI install needs attention. Run migration update in terminal?', { modal: true }, 'Run update', 'Later');
			if (choice === 'Run update') {
				openInstallerTerminal('LiveReview lrc migration', platformInstallCommand());
			}
			return status;
		}

		const newLocalVersion = await getLocalLrcVersion(resolveLrcPath);
		const status: LrcUpdateStatus = {
			state: 'updated',
			message: `lrc installed successfully (${newLocalVersion ?? 'installed'}).`,
			localVersion: newLocalVersion
		};
		emit(status);
		if (opts?.notifyOnBackgroundSuccess) {
			void vscode.window.showInformationMessage('LiveReview: lrc installed in background.');
		}
		return status;
	}

	if (!remoteVersion) {
		const status: LrcUpdateStatus = {
			state: localVersion ? 'upToDate' : 'failed',
			message: localVersion
				? `Could not determine latest lrc version from release manifest; keeping current version (${localVersion}).`
				: 'Could not determine latest lrc version from release manifest and lrc is not installed.',
			localVersion
		};
		emit(status);
		return status;
	}

	if (!updateNeeded) {
		const status: LrcUpdateStatus = {
			state: 'upToDate',
			message: `lrc is up to date (${localVersion}).`,
			localVersion,
			remoteVersion
		};
		emit(status);
		return status;
	}

	if (opts?.checkOnly) {
		const status: LrcUpdateStatus = {
			state: legacyMigrationRequired ? 'needsMigration' : 'updateAvailable',
			message: legacyMigrationRequired
				? `lrc update requires migration (${currentLabel} → ${remoteVersion}).`
				: `lrc update available (${currentLabel} → ${remoteVersion}).`,
			localVersion,
			remoteVersion
		};
		emit(status);
		return status;
	}

	if (legacyMigrationRequired) {
		const status: LrcUpdateStatus = {
			state: 'needsMigration',
			message: `Legacy binaries detected; migration update needed (${currentLabel} → ${remoteVersion}).`,
			localVersion,
			remoteVersion
		};
		emit(status);
		const message = `LiveReview CLI update requires migration (${currentLabel} → ${remoteVersion}). Run update now?`;
		const choice = await vscode.window.showInformationMessage(message, { modal: true }, 'Run update', 'Later');
		if (choice === 'Run update') {
			emit({ state: 'updating', message: `Running migration update in terminal (${currentLabel} → ${remoteVersion}).`, localVersion, remoteVersion });
			openInstallerTerminal('LiveReview lrc migration', platformInstallCommand());
		}
		return status;
}

	emit({
		state: 'updating',
		message: `Running rootless background update (${currentLabel} → ${remoteVersion}).`,
		localVersion,
		remoteVersion
	});

	const backgroundResult = await runBackgroundInstaller(opts?.backgroundTimeoutMs ?? 120000);
	if (backgroundResult === 'timeout') {
		const status: LrcUpdateStatus = {
			state: 'needsMigration',
			message: `Background update timed out (${currentLabel} → ${remoteVersion}). Migration update may be required.`,
			localVersion,
			remoteVersion
		};
		emit(status);
		const choice = await vscode.window.showInformationMessage('LiveReview CLI update needs attention. Run migration update in terminal?', { modal: true }, 'Run update', 'Later');
		if (choice === 'Run update') {
			openInstallerTerminal('LiveReview lrc migration', platformInstallCommand());
		}
		return status;
}

	const refreshedLocalVersion = await getLocalLrcVersion(resolveLrcPath);
	const status: LrcUpdateStatus = {
		state: 'updated',
		message: `lrc updated successfully (${refreshedLocalVersion ?? remoteVersion}).`,
		localVersion: refreshedLocalVersion,
		remoteVersion
	};
	emit(status);
	if (opts?.notifyOnBackgroundSuccess) {
		void vscode.window.showInformationMessage(`LiveReview: lrc updated to ${refreshedLocalVersion ?? remoteVersion}.`);
	}
	return status;
};

const fetchLatestExtensionVersion = async (): Promise<string | undefined> => {
	const url = new URL('https://open-vsx.org/api/Hexmos/livereview');
	const options: https.RequestOptions = {
		method: 'GET',
		hostname: url.hostname,
		path: `${url.pathname}${url.search}`
	};
	try {
		const raw = await httpRequest(options);
		const data = JSON.parse(raw) as { version?: string };
		return data.version ? semverFromString(data.version) ?? data.version : undefined;
	} catch {
		return undefined;
	}
};

export const ensureLatestExtension = async (context: vscode.ExtensionContext, output: vscode.OutputChannel): Promise<void> => {
	const localVersion = semverFromString(context.extension.packageJSON.version) ?? context.extension.packageJSON.version;
	const remoteVersion = await fetchLatestExtensionVersion();
	if (!remoteVersion) {
		output.appendLine('LiveReview: Could not determine latest extension version; skipping update prompt.');
		return;
	}

	if (semverCompare(localVersion, remoteVersion) >= 0) {
		output.appendLine(`LiveReview: extension is up to date (${localVersion}).`);
		return;
	}

	const message = `LiveReview extension is outdated (${localVersion} → ${remoteVersion}). Update in Extensions view?`;
	const choice = await vscode.window.showInformationMessage(message, { modal: true }, 'Open Extensions View', 'Later');
	if (choice === 'Open Extensions View') {
		await vscode.commands.executeCommand('workbench.extensions.search', 'publisher:Hexmos livereview');
	}
};
