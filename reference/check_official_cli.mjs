import { accessSync, constants } from "node:fs";
import { spawnSync } from "node:child_process";

const binary = process.argv[2] || process.env.RGB_OFFICIAL_CLI;
if (!binary) {
  console.error("usage: node reference/check_official_cli.mjs <path-to-rgb>");
  process.exit(2);
}

accessSync(binary, constants.X_OK);

function run(args) {
  const result = spawnSync(binary, args, { encoding: "utf8" });
  const output = `${result.stdout || ""}${result.stderr || ""}`;
  if (result.status !== 0) {
    throw new Error(`${binary} ${args.join(" ")} failed (${result.status}):\n${output}`);
  }
  return output;
}

const version = run(["--version"]).trim();
if (version !== "rgb-wallet 0.11.1-alpha.3+unreviewed") {
  throw new Error(`unexpected frozen RGB CLI version: ${version}`);
}

const rootHelp = run(["--help"]);
const commands = [
  "invoice",
  "transfer",
  "inspect",
  "validate",
  "accept",
  "finalize",
];
for (const command of commands) {
  if (!new RegExp(`^  ${command}\\s`, "m").test(rootHelp)) {
    throw new Error(`frozen RGB CLI is missing required command: ${command}`);
  }
  run([command, "--help"]);
}

if (/^  check\s/m.test(rootHelp)) {
  throw new Error("frozen RGB CLI unexpectedly exposes the legacy check command");
}

console.log(
  JSON.stringify(
    {
      status: "ok",
      binary,
      version,
      commands,
      legacyCheckCommand: false,
    },
    null,
    2,
  ),
);
