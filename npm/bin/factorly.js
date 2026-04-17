#!/usr/bin/env node
/**
 * Copyright 2026 Jordan Sherer <hi@jordansherer.com>
 * SPDX-License-Identifier: gpl
 */

"use strict";

const { spawnSync } = require("child_process");
const path = require("path");

const IS_WIN = process.platform === "win32";
const BINARY = path.join(__dirname, IS_WIN ? "factorly.exe" : "factorly");

const result = spawnSync(BINARY, process.argv.slice(2), {
  stdio: "inherit",
  env: process.env,
});

if (result.error) {
  if (result.error.code === "ENOENT") {
    console.error("factorly binary not found. Run: npm rebuild factorly");
  } else {
    console.error(`factorly: ${result.error.message}`);
  }
  process.exit(127);
}

process.exit(result.status ?? 1);
