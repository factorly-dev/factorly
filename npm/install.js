#!/usr/bin/env node
/**
 * Copyright 2026 Jordan Sherer <hi@jordansherer.com>
 * SPDX-License-Identifier: gpl
 */


const https = require("https");
const http = require("http");
const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");

const VERSION = require("./package.json").version;
const REPO = "factorly-dev/factorly";
const BIN_DIR = path.join(__dirname, "bin");
const BIN_PATH = path.join(BIN_DIR, process.platform === "win32" ? "factorly.exe" : "factorly");

const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

function getBinaryName() {
  const platform = PLATFORM_MAP[process.platform];
  const arch = ARCH_MAP[process.arch];

  if (!platform) {
    throw new Error(`Unsupported platform: ${process.platform}`);
  }
  if (!arch) {
    throw new Error(`Unsupported architecture: ${process.arch}`);
  }

  const ext = process.platform === "win32" ? ".exe" : "";
  return `factorly-${VERSION}-${platform}-${arch}${ext}`;
}

function download(url) {
  return new Promise((resolve, reject) => {
    const client = url.startsWith("https") ? https : http;
    client
      .get(url, { headers: { "User-Agent": "factorly-npm-installer" } }, (res) => {
        // Follow redirects (GitHub releases redirect to S3)
        if (res.statusCode === 301 || res.statusCode === 302) {
          return download(res.headers.location).then(resolve).catch(reject);
        }
        if (res.statusCode !== 200) {
          return reject(new Error(`Download failed: HTTP ${res.statusCode} from ${url}`));
        }
        const chunks = [];
        res.on("data", (chunk) => chunks.push(chunk));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      })
      .on("error", reject);
  });
}

async function install() {
  // Skip if binary already exists (e.g., from a previous install)
  if (fs.existsSync(BIN_PATH)) {
    try {
      const output = execSync(`"${BIN_PATH}" version`, { encoding: "utf8" }).trim();
      if (output.includes(VERSION)) {
        console.log(`factorly ${VERSION} already installed.`);
        return;
      }
    } catch {
      // Binary exists but doesn't work — re-download
    }
  }

  const binaryName = getBinaryName();
  const url = `https://github.com/${REPO}/releases/download/v${VERSION}/${binaryName}`;

  console.log(`Downloading factorly ${VERSION} for ${process.platform}/${process.arch}...`);
  console.log(`  ${url}`);

  const data = await download(url);

  // Ensure bin directory exists
  fs.mkdirSync(BIN_DIR, { recursive: true });

  // Write binary
  fs.writeFileSync(BIN_PATH, data, { mode: 0o755 });

  console.log(`Installed factorly to ${BIN_PATH}`);
}

install().catch((err) => {
  console.error(`Failed to install factorly: ${err.message}`);
  console.error("");
  console.error("You can install manually from:");
  console.error(`  https://github.com/${REPO}/releases/tag/v${VERSION}`);
  console.error("");
  console.error("Or build from source:");
  console.error("  git clone https://github.com/factorly-dev/factorly.git");
  console.error("  cd factorly && make build");
  process.exit(1);
});
