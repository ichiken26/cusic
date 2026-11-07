import { test } from "node:test";
import { strict as assert } from "node:assert";
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";

test("generates an ES5-compatible SynthV script from validated data", () => {
  const dir = mkdtempSync(join(tmpdir(), "cusic-"));
  try {
    const input = join(dir, "notes.json");
    const output = join(dir, "import.js");
    writeFileSync(input, JSON.stringify({
      version: 1, trackId: "t", trackName: "Teto", ppq: 480,
      fromTick: 0, toTick: 1920, notes: [
        { id: "n", trackId: "t", tick: 0, duration: 480, pitch: 60, velocity: 100, lyric: "あ" }
      ]
    }));
    const result = spawnSync(process.execPath, ["dist/host/prepare.js", input, "--out", output], {encoding:"utf8"});
    assert.equal(result.status, 0, result.stderr);
    const script = readFileSync(output, "utf8");
    assert.ok(script.includes("Teto") && script.includes("SV.getMainEditor"));
    assert.ok(!script.includes("__CUSIC_PAYLOAD__"));
  } finally { rmSync(dir, { recursive: true, force: true }); }
});
test("rejects invalid target boundaries", () => {
  const dir = mkdtempSync(join(tmpdir(), "cusic-"));
  try {
    const input = join(dir, "bad.json");
    writeFileSync(input, JSON.stringify({
      version:1, trackId:"t",trackName:"Teto",ppq:480,fromTick:1000,toTick:500,notes:[]
    }));
    const result = spawnSync(process.execPath, ["dist/host/prepare.js", input, "--out", join(dir,"bad.js")], {encoding:"utf8"});
    assert.notEqual(result.status, 0);
  } finally { rmSync(dir, { recursive: true, force: true }); }
});
