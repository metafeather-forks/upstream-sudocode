/**
 * .sudocode file/dir indirection — follows Git's gitdir: pattern.
 *
 * When .sudocode is a directory: data lives there (co-located mode).
 * When .sudocode is a file: contains a single line `sudocodedir: <path>` pointing to the data directory.
 */

import * as fs from "fs";
import * as path from "path";

const FILE_NAME = ".sudocode";
const PREFIX = "sudocodedir: ";

/**
 * Resolve the sudocode data directory for a repository.
 *
 * - If .sudocode is a directory, returns its absolute path.
 * - If .sudocode is a file, reads the `sudocodedir:` line and resolves the path.
 * - If .sudocode does not exist, returns null.
 * - If .sudocode file is malformed, throws an error.
 *
 * Relative paths in the file are resolved relative to repoPath.
 */
export function resolveSudocodeDir(repoPath: string): string | null {
  const p = path.join(repoPath, FILE_NAME);

  let stat: fs.Stats;
  try {
    stat = fs.statSync(p);
  } catch {
    return null;
  }

  if (stat.isDirectory()) {
    return p;
  }

  // It's a file — read and parse
  const content = fs.readFileSync(p, "utf8").replace(/[\r\n]+$/, "");

  if (!content.startsWith(PREFIX)) {
    throw new Error(
      `Malformed .sudocode file: missing "${PREFIX}" prefix`
    );
  }

  const dir = content.slice(PREFIX.length);
  if (!dir) {
    throw new Error("Malformed .sudocode file: empty path after prefix");
  }

  if (path.isAbsolute(dir)) {
    return path.normalize(dir);
  }

  return path.resolve(repoPath, dir);
}

/**
 * Write a .sudocode file with a `sudocodedir:` line pointing to the given directory.
 */
export function writeSudocodeFile(
  repoPath: string,
  sudocodeDir: string
): void {
  const p = path.join(repoPath, FILE_NAME);
  fs.writeFileSync(p, `${PREFIX}${sudocodeDir}\n`, "utf8");
}
