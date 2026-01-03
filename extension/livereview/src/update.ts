import * as https from 'https';
import * as util from 'util';
import * as vscode from 'vscode';
import { execFile } from 'child_process';

const execFileAsync = util.promisify(execFile);

// Backblaze B2 constants (read-only credentials)
const B2_KEY_ID = '00536b4c5851afd0000000006';
const B2_APP_KEY = 'K005DV+hNk6/fdQr8oXHmRsdo8U2YAU';
const B2_BUCKET_ID = '33d6ab74ac456875919a0f1d';
const B2_PREFIX = 'lrc';
const B2_AUTH_URL = 'https://api.backblazeb2.com/b2api/v2/b2_authorize_account';

const VERSION_CACHE_TTL_MS = 5 * 60 * 1000;

interface B2CacheEntry {
	latestVersion: string;
	expiresAt: number;
}

let b2Cache: B2CacheEntry | undefined;

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

const fetchLatestLrcVersionFromB2 = async (forceRefresh = false): Promise<string | undefined> => {
	const now = Date.now();
	if (!forceRefresh && b2Cache && b2Cache.expiresAt > now) {
		return b2Cache.latestVersion;
	}

	const authUrl = new URL(B2_AUTH_URL);
	const authOptions: https.RequestOptions = {
		method: 'GET',
		headers: {
			Authorization: `Basic ${Buffer.from(`${B2_KEY_ID}:${B2_APP_KEY}`).toString('base64')}`
		},
		hostname: authUrl.hostname,
		path: `${authUrl.pathname}${authUrl.search}`
	};

	const authResponseRaw = await httpRequest(authOptions);
	const authResponse = JSON.parse(authResponseRaw) as { authorizationToken?: string; apiUrl?: string };
	const authToken = authResponse.authorizationToken;
	const apiUrl = authResponse.apiUrl;
	if (!authToken || !apiUrl) {
		return undefined;
	}

	const listBody = JSON.stringify({
		bucketId: B2_BUCKET_ID,
		startFileName: `${B2_PREFIX}/`,
		prefix: `${B2_PREFIX}/`,
		maxFileCount: 1000
	});

	const listUrl = new URL('/b2api/v2/b2_list_file_names', apiUrl);
	const listOptions: https.RequestOptions = {
		method: 'POST',
		headers: {
			Authorization: authToken,
			'Content-Type': 'application/json',
			'Content-Length': Buffer.byteLength(listBody)
		},
		hostname: listUrl.hostname,
		path: `${listUrl.pathname}${listUrl.search}`
	};

	const listResponseRaw = await httpRequest(listOptions, listBody);
	const listResponse = JSON.parse(listResponseRaw) as { files?: Array<{ fileName?: string }> };
	const versions = (listResponse.files ?? [])
		.map(f => f.fileName ?? '')
		.map(name => {
			const match = name.match(/^lrc\/(v\d+\.\d+\.\d+)\//);
			return match ? match[1] : undefined;
		})
		.filter((v): v is string => Boolean(v));

	const latest = versions.reduce<string | undefined>((best, cur) => {
		if (!best) { return cur; }
		return semverCompare(cur, best) > 0 ? cur : best;
	}, undefined);

	if (latest) {
		b2Cache = { latestVersion: latest, expiresAt: now + VERSION_CACHE_TTL_MS };
	}
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
	return 'curl -fsSL https://hexmos.com/lrc-install.sh | sudo bash';
};

const openInstallerTerminal = (title: string, command: string) => {
	const term = vscode.window.createTerminal({ name: title });
	term.show(true);
	term.sendText(command, true);
};

export const ensureLatestLrc = async (
	resolveLrcPath: () => Promise<string>,
	output: vscode.OutputChannel,
	opts?: { forceRemoteRefresh?: boolean }
): Promise<void> => {
	const localVersion = await getLocalLrcVersion(resolveLrcPath);
	const remoteVersion = await fetchLatestLrcVersionFromB2(opts?.forceRemoteRefresh ?? false);

	if (!remoteVersion && !localVersion) {
		const installCmd = platformInstallCommand();
		const message = 'LiveReview CLI (lrc) is not installed. Install now? The installer may request sudo/administrator permission.';
		const choice = await vscode.window.showInformationMessage(message, { modal: true }, 'Install now', 'Later');
		if (choice === 'Install now') {
			output.appendLine('LiveReview: installing lrc (no local version found).');
			openInstallerTerminal('LiveReview lrc install', installCmd);
		} else {
			output.appendLine('LiveReview: lrc installation skipped; functionality may be limited.');
		}
		return;
	}

	if (!remoteVersion) {
		output.appendLine('LiveReview: Could not determine latest lrc version from B2; skipping update check.');
		return;
	}

	if (localVersion && semverCompare(localVersion, remoteVersion) >= 0) {
		output.appendLine(`LiveReview: lrc is up to date (${localVersion}).`);
		return;
	}

	const currentLabel = localVersion ?? 'not installed';
	const installCmd = platformInstallCommand();
	const message = `LiveReview CLI is outdated (${currentLabel} → ${remoteVersion}). Update now? The installer may request sudo/administrator permission.`;
	const choice = await vscode.window.showInformationMessage(message, { modal: true }, 'Update now', 'Later');
	if (choice !== 'Update now') {
		output.appendLine('LiveReview: lrc update skipped by user; functionality may be limited.');
		return;
	}

	output.appendLine(`LiveReview: running installer to update lrc (${currentLabel} → ${remoteVersion}).`);
	openInstallerTerminal('LiveReview lrc update', installCmd);
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
