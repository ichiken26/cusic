import { readFileSync, writeFileSync, mkdirSync, existsSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { payloadSchema } from "./schema.js";

// Syntax: node dist/host/prepare.js ./teto.json --out ./cusic-teto.js
const args = process.argv.slice(2);
const input = args[0];
const outFlag = args.indexOf("--out");
const output = outFlag >= 0 ? args[outFlag + 1] : undefined;
if (!input || !output) {
  console.error("usage: node dist/host/prepare.js PAYLOAD.json --out SCRIPT.js");
  process.exit(2);
}
const parsed = payloadSchema.safeParse(JSON.parse(readFileSync(input, "utf8")));
if (!parsed.success) {
  console.error(parsed.error.issues.map(i => i.path.join(".") + ": " + i.message).join("\n"));
  process.exit(1);
}
const base = resolve(dirname(fileURLToPath(import.meta.url)), "../cusic-synthv.js");
const template = readFileSync(base, "utf8");
const needle = JSON.stringify("__CUSIC_PAYLOAD__");
if (!template.includes(needle) || template.indexOf(needle) !== template.lastIndexOf(needle)) {
  throw new Error("Unexpected cusic bridge template format");
}
const built = template.replace(needle, JSON.stringify(JSON.stringify(parsed.data)));
if (existsSync(output)) throw new Error("Output script already exists; choose a new filename.");
mkdirSync(dirname(resolve(output)), { recursive: true });
writeFileSync(output, built, "utf8");
console.log("Created validated SynthV script: " + output);
