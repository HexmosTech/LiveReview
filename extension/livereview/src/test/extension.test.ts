import * as assert from 'assert';
import * as fs from 'fs';
import * as path from 'path';

// You can import and use all API from the 'vscode' module
// as well as import your extension to test it
import * as vscode from 'vscode';
// import * as myExtension from '../../extension';

suite('Extension Test Suite', () => {
	vscode.window.showInformationMessage('Start all tests.');

	test('Sample test', () => {
		assert.strictEqual(-1, [1, 2, 3].indexOf(5));
		assert.strictEqual(-1, [1, 2, 3].indexOf(0));
	});

	test('Extension does not define settings-to-file sync writer for ~/.lrc.toml', () => {
		const sourcePath = path.resolve(__dirname, '../../src/extension.ts');
		const source = fs.readFileSync(sourcePath, 'utf8');

		assert.ok(!source.includes('const syncFileFromSettings = async'), 'syncFileFromSettings should not exist');
		assert.ok(!source.includes('const writeLrcConfig = async'), 'writeLrcConfig should not exist');
		assert.ok(!source.includes('writeFile(lrcConfigPath'), 'direct writeFile(lrcConfigPath, ...) should not exist');
	});
});
