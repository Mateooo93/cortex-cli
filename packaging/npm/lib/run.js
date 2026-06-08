"use strict";

const { spawn } = require("child_process");
const fs = require("fs");

const { ensureBinary } = require("./install");
const { name: packageName } = require("../package.json");

async function main() {
  let binaryPath;
  try {
    binaryPath = await ensureBinary();
  } catch (err) {
    console.error(err.message || err);
    process.exit(1);
  }

  if (!binaryPath || !fs.existsSync(binaryPath)) {
    console.error(
      `cortex-cli: native binary not installed. Re-run: npm install -g ${packageName} --registry=https://npm.pkg.github.com`
    );
    process.exit(1);
  }

  const env = {
    ...process.env,
    CORTEX_NPM_PACKAGE: packageName,
    CORTEX_NPM_SHIM: __filename,
  };

  const child = spawn(binaryPath, process.argv.slice(2), {
    stdio: "inherit",
    windowsHide: false,
    env,
  });

  child.on("error", (err) => {
    console.error(`cortex-cli: failed to launch ${binaryPath}: ${err.message}`);
    process.exit(1);
  });

  child.on("exit", (code, signal) => {
    if (signal) {
      process.kill(process.pid, signal);
      return;
    }
    process.exit(code ?? 1);
  });
}

main();