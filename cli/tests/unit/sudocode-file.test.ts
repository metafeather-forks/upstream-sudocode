import { describe, it, expect, beforeEach, afterEach } from "vitest";
import * as fs from "fs";
import * as path from "path";
import * as os from "os";
import { resolveSudocodeDir, writeSudocodeFile } from "../../src/sudocode-file.js";

function makeTmpDir(): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), "sudocode-test-"));
}

function cleanup(dir: string): void {
  fs.rmSync(dir, { recursive: true, force: true });
}

describe("resolveSudocodeDir", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
  });

  afterEach(() => {
    cleanup(tmpDir);
  });

  it("returns directory path when .sudocode is a directory", () => {
    const sudocodeDir = path.join(tmpDir, ".sudocode");
    fs.mkdirSync(sudocodeDir);

    expect(resolveSudocodeDir(tmpDir)).toBe(sudocodeDir);
  });

  it("resolves absolute path from .sudocode file", () => {
    const targetDir = makeTmpDir();
    fs.writeFileSync(
      path.join(tmpDir, ".sudocode"),
      `sudocodedir: ${targetDir}\n`
    );

    expect(resolveSudocodeDir(tmpDir)).toBe(targetDir);
    cleanup(targetDir);
  });

  it("resolves relative path from .sudocode file", () => {
    const targetDir = path.join(tmpDir, "shared", ".sudocode-data");
    fs.mkdirSync(targetDir, { recursive: true });
    fs.writeFileSync(
      path.join(tmpDir, ".sudocode"),
      "sudocodedir: shared/.sudocode-data\n"
    );

    expect(resolveSudocodeDir(tmpDir)).toBe(targetDir);
  });

  it("returns null when .sudocode does not exist", () => {
    expect(resolveSudocodeDir(tmpDir)).toBeNull();
  });

  it("throws on empty file", () => {
    fs.writeFileSync(path.join(tmpDir, ".sudocode"), "");
    expect(() => resolveSudocodeDir(tmpDir)).toThrow("Malformed");
  });

  it("throws on missing prefix", () => {
    fs.writeFileSync(path.join(tmpDir, ".sudocode"), "/some/path\n");
    expect(() => resolveSudocodeDir(tmpDir)).toThrow("Malformed");
  });

  it("throws on wrong prefix", () => {
    fs.writeFileSync(path.join(tmpDir, ".sudocode"), "gitdir: /some/path\n");
    expect(() => resolveSudocodeDir(tmpDir)).toThrow("Malformed");
  });

  it("throws on prefix with empty path", () => {
    fs.writeFileSync(path.join(tmpDir, ".sudocode"), "sudocodedir: \n");
    expect(() => resolveSudocodeDir(tmpDir)).toThrow("empty path");
  });
});

describe("writeSudocodeFile", () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = makeTmpDir();
  });

  afterEach(() => {
    cleanup(tmpDir);
  });

  it("writes correct format", () => {
    const target = "/some/external/.sudocode/projects/my-project";
    writeSudocodeFile(tmpDir, target);

    const content = fs.readFileSync(path.join(tmpDir, ".sudocode"), "utf8");
    expect(content).toBe(`sudocodedir: ${target}\n`);
  });

  it("overwrites existing file", () => {
    fs.writeFileSync(
      path.join(tmpDir, ".sudocode"),
      "sudocodedir: /old/path\n"
    );
    writeSudocodeFile(tmpDir, "/new/path");

    const content = fs.readFileSync(path.join(tmpDir, ".sudocode"), "utf8");
    expect(content).toBe("sudocodedir: /new/path\n");
  });
});
