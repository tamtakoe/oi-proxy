#!/usr/bin/env node

import { argv } from 'node:process';
import { spawn } from 'node:child_process';
import { resolveBinaryPath } from './resolve-binary.js';

const binaryPath = resolveBinaryPath();

const child = spawn(binaryPath, argv.slice(2), {
	stdio: 'inherit'
});

child.on('exit', (code, signal) => {
	if (signal) {
		process.kill(process.pid, signal);
	} else {
		process.exit(code ?? 0);
	}
});

