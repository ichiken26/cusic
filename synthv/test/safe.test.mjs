import {test} from "node:test";
import {strict as assert} from "node:assert";
import {mkdtempSync,rmSync,readFileSync,writeFileSync} from "node:fs";
import {tmpdir} from "node:os";
import {join} from "node:path";
import {spawnSync} from "node:child_process";
test("safe edit compiles to independently runnable JS with no Zod runtime",()=>{
 const tmp=mkdtempSync(join(tmpdir(),"cusic-safe-"));
 try{
  const payload=join(tmp,"edits.json"),out=join(tmp,"safe.js");
  writeFileSync(payload,JSON.stringify({version:1,trackName:"Teto",ppq:480,changes:[{tick:480,expectedPitch:60,newPitch:62}]}));
  const result=spawnSync(process.execPath,["dist/host/prepare-safe.js",payload,"--out",out],{encoding:"utf8"});
  assert.equal(result.status,0,result.stderr);
  const script=readFileSync(out,"utf8");
  assert.ok(script.includes("Teto")&&script.includes("getPitch()"));
  assert.ok(!script.includes("__CUSIC_SAFE_PAYLOAD__"));
 }finally{rmSync(tmp,{recursive:true,force:true});}
});
test("safe edit rejects duplicate target",()=>{
 const tmp=mkdtempSync(join(tmpdir(),"cusic-safe-"));
 try{
  const payload=join(tmp,"edits.json");
  writeFileSync(payload,JSON.stringify({version:1,trackName:"Teto",ppq:480,changes:[
   {tick:480,expectedPitch:60,newPitch:62},{tick:480,expectedPitch:60,newPitch:65}]}));
  const result=spawnSync(process.execPath,["dist/host/prepare-safe.js",payload,"--out",join(tmp,"bad.js")],{encoding:"utf8"});
  assert.notEqual(result.status,0);
 }finally{rmSync(tmp,{recursive:true,force:true});}
});
