#!/usr/bin/env node

import { resolveBinaryPath } from './resolve-binary.js';

try {
	const binaryPath = resolveBinaryPath();
	console.log(`[oi-proxy] Using binary: ${binaryPath}`);
} catch (error) {
	console.error(`[oi-proxy] ${error.message}`);
	process.exit(1);
}

