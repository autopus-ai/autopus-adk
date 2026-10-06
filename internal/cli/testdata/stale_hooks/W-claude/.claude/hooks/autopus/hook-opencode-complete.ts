// Autopus orchestra result collector — OpenCode text.complete plugin.
import { randomBytes } from "crypto";
import {
  closeSync,
  constants,
  fchmodSync,
  fstatSync,
  fsyncSync,
  lstatSync,
  openSync,
  readFileSync,
  renameSync,
  unlinkSync,
  writeFileSync,
} from "fs";
import { isAbsolute, join } from "path";

const sessId = process.env.AUTOPUS_SESSION_ID;
if (!sessId || !/^[a-zA-Z0-9_-]+$/.test(sessId)) process.exit(0);

const sessDir = process.env.AUTOPUS_SESSION_DIR || join("/tmp/autopus", sessId);
if (!isAbsolute(sessDir)) process.exit(0);
try {
  const info = lstatSync(sessDir);
  if (!info.isDirectory() || info.isSymbolicLink()) process.exit(0);
} catch {
  process.exit(0);
}

const cursorFile = "opencode-round-cursor";
const maxCursorBytes = 64;
const maxInputBytes = 1024 * 1024;
const noFollow = constants.O_NOFOLLOW ?? 0;
const nonBlock = constants.O_NONBLOCK ?? 0;

function artifactPath(name: string): string {
  if (name === "." || name === ".." || !/^[a-zA-Z0-9._-]+$/.test(name)) {
    throw new Error("unsafe artifact name");
  }
  return join(sessDir, name);
}

function atomicWrite(name: string, body: string | Buffer): void {
  const temporary = artifactPath(`.autopus-hook-${randomBytes(12).toString("hex")}`);
  let fd = -1;
  try {
    fd = openSync(temporary, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | noFollow, 0o600);
    writeFileSync(fd, body);
    fchmodSync(fd, 0o600);
    fsyncSync(fd);
    closeSync(fd);
    fd = -1;
    renameSync(temporary, artifactPath(name));
  } catch (error) {
    if (fd >= 0) {
      try { closeSync(fd); } catch {}
    }
    try { unlinkSync(temporary); } catch {}
    throw error;
  }
}

function readRegular(name: string, maxBytes: number, encoding: BufferEncoding): string {
  let fd = -1;
  try {
    fd = openSync(artifactPath(name), constants.O_RDONLY | nonBlock | noFollow);
    const info = fstatSync(fd);
    if (!info.isFile() || info.size > maxBytes) throw new Error("unsafe artifact");
    const body = readFileSync(fd);
    if (body.length > maxBytes) throw new Error("oversized artifact");
    return body.toString(encoding);
  } finally {
    if (fd >= 0) closeSync(fd);
  }
}

function artifactExists(name: string): boolean {
  try {
    lstatSync(artifactPath(name));
    return true;
  } catch {
    return false;
  }
}

function removeArtifact(name: string): void {
  try { unlinkSync(artifactPath(name)); } catch {}
}

function parseRound(value: string | undefined, allowNewline = false): number | null {
  const candidate = allowNewline ? value?.replace(/[\r\n]+$/, "") : value;
  if (!candidate || !/^[0-9]+$/.test(candidate)) return null;
  const round = Number(candidate);
  if (!Number.isSafeInteger(round) || round > 2147483646) return null;
  return round;
}

function readCursorRound(): number | null {
  try {
    return parseRound(readRegular(cursorFile, maxCursorBytes, "ascii"), true);
  } catch {
    return null;
  }
}

const chunks: Buffer[] = [];
process.stdin.on("data", (chunk) => chunks.push(Buffer.from(chunk)));
process.stdin.on("end", () => {
  const input = Buffer.concat(chunks).toString();
  let text = "";
  try {
    const data = JSON.parse(input);
    text = typeof data.text === "string" ? data.text : "";
  } catch {
    text = input;
  }
  if (!text) process.exit(0);

  const envRound = parseRound(process.env.AUTOPUS_ROUND);
  const cursorRound = readCursorRound();
  const effectiveRound = Math.max(envRound ?? -1, cursorRound ?? -1);
  const validRound = effectiveRound >= 0 ? String(effectiveRound) : null;
  const suffix = validRound ? `-round${validRound}` : "";
  const resultFile = `opencode${suffix}-result.json`;
  const doneFile = `opencode${suffix}-done`;
  try {
    atomicWrite(resultFile, JSON.stringify({ output: text, exit_code: 0 }));
    atomicWrite(doneFile, "");
  } catch {
    process.exit(0);
  }

  if (!validRound) return;
  const nextRound = String(Number(validRound) + 1);
  const readyFile = `opencode-round${nextRound}-ready`;
  const inputFile = `opencode-round${nextRound}-input.json`;
  const abortFile = `opencode-round${nextRound}-abort`;
  try {
    atomicWrite(readyFile, "");
  } catch {
    process.exit(0);
  }

  let waited = 0;
  const maxWait = 120000;
  const poll = setInterval(() => {
    waited += 200;
    if (artifactExists(abortFile)) {
      clearInterval(poll);
      removeArtifact(readyFile);
      removeArtifact(abortFile);
      process.exit(0);
    }

    if (artifactExists(inputFile)) {
      clearInterval(poll);
      let prompt = "";
      try {
        const parsed = JSON.parse(readRegular(inputFile, maxInputBytes, "utf-8"));
        const validInput = parsed.provider === "opencode" && parsed.round === Number(nextRound);
        prompt = validInput && typeof parsed.prompt === "string" ? parsed.prompt : "";
      } catch {}
      if (prompt) {
        try {
          atomicWrite(cursorFile, `${nextRound}\n`);
        } catch {
          removeArtifact(inputFile);
          removeArtifact(readyFile);
          process.exit(0);
        }
      }
      removeArtifact(inputFile);
      removeArtifact(readyFile);
      if (prompt) process.stdout.write(prompt);
      process.exit(0);
    }

    if (waited >= maxWait) {
      clearInterval(poll);
      removeArtifact(readyFile);
      process.exit(0);
    }
  }, 200);
});
