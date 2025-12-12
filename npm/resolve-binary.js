import { createRequire } from 'node:module';
import { existsSync } from 'node:fs';

const require = createRequire(import.meta.url);

const PLATFORM_PACKAGES = {
	darwin: {
		arm64: '@oi-proxy/proxy-darwin-arm64',
		x64: '@oi-proxy/proxy-darwin-amd64'
	},
	linux: {
		x64: '@oi-proxy/proxy-linux-amd64'
	},
	win32: {
		x64: '@oi-proxy/proxy-win32-amd64'
	}
};

export function resolveBinaryPath() {
	const platformTargets = PLATFORM_PACKAGES[process.platform];
	if (!platformTargets) {
		throw new Error(`Unsupported platform: ${process.platform}`);
	}

	const packageName = platformTargets[process.arch];
	if (!packageName) {
		throw new Error(`Unsupported architecture: ${process.platform}/${process.arch}`);
	}

	let binaryPath;
	try {
		binaryPath = require(packageName);
	} catch (error) {
		throw new Error(
			`Failed to load optional dependency "${packageName}". Run "npm install oi-proxy" again or publish the platform package. Original error: ${error.message}`
		);
	}

	if (typeof binaryPath !== 'string') {
		throw new Error(`Package "${packageName}" did not export a binary path`);
	}

	if (!existsSync(binaryPath)) {
		throw new Error(`Binary not found at "${binaryPath}". Try reinstalling the package.`);
	}

	return binaryPath;
}

