import { readFileSync, writeFileSync, mkdirSync, existsSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { safeEditSchema } from "./schema.js";

const args=process.argv.slice(2);
const flag=args.indexOf("--out");
const input=args[0],output=flag>=0?args[flag+1]:undefined;
if(!input||!output){console.error("usage: node dist/host/prepare-safe.js EDITS.json --out NEW-SCRIPT.js");process.exit(2);}
const parsed=safeEditSchema.safeParse(JSON.parse(readFileSync(input,"utf8")));
if(!parsed.success){
 console.error(parsed.error.issues.map(i=>i.path.join(".")+": "+i.message).join("\n"));
 process.exit(1);
}
const dir=resolve(dirname(fileURLToPath(import.meta.url)),"..");
const script=readFileSync(resolve(dir,"cusic-synthv-safe.js"),"utf8");
const needle=JSON.stringify("__CUSIC_SAFE_PAYLOAD__");
if(script.indexOf(needle)<0||script.indexOf(needle)!==script.lastIndexOf(needle)){
 throw new Error("Unexpected TypeScript build artifact");
}
if(existsSync(output))throw new Error("Output exists; choose a new name");
const result=script.replace(needle,JSON.stringify(JSON.stringify(parsed.data)));
mkdirSync(dirname(resolve(output)),{recursive:true});
writeFileSync(output,result,"utf8");
console.log("Prepared NON-DESTRUCTIVE SynthV script: "+output);
