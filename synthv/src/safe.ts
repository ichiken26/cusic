/*
 * Synthesizer V 2 Pro: non-destructive existing-note edits.
 * Developed in TypeScript; compile to standalone ES5 JS before running.
 * Preserve existing note objects, manual parameter curves, vocal modes and
 * retakes; never remove or create notes.
 */
type SafeChange = {
 tick: number;
 expectedPitch: number;
 newPitch?: number;
 newLyric?: string;
};
type SafePayload = {version: number; trackName: string; ppq: 480; changes: SafeChange[]};
function getClientInfo() {
 return {name:"cusic: Safe Edit Existing Notes",category:"cusic",
  author:"cusic contributors",versionNumber:1,minEditorVersion:65540};
}
function main() {
 try {
  var payload = JSON.parse("__CUSIC_SAFE_PAYLOAD__") as SafePayload;
  if (!payload || payload.version!==1 || payload.ppq!==480) {
   throw new Error("Prepared cusic safe edit script required.");
  }
  var editor = SV.getMainEditor();
  var track = editor.getCurrentTrack();
  if (!track || track.getName()!==payload.trackName){
   throw new Error("Select the SynthV track named " + payload.trackName);
  }
  var reference = editor.getCurrentGroup();
  if (!reference || !reference.isMain()) {
   throw new Error("Open the track's main note group.");
  }
  var group = reference.getTarget();
  var offset = reference.getTimeOffset();
  // Resolve and preflight every requested note before creating an undo record.
  var indexes:number[] = [];
  var i:number,j:number;
  for (i=0;i<payload.changes.length;i++) {
   var change = payload.changes[i];
   var onset = SV.quarter2Blick(change.tick/payload.ppq)-offset;
   var matched = -1;
   for (j=0;j<group.getNumNotes();j++){
    var candidate = group.getNote(j);
    if (Math.abs(candidate.getOnset()-onset)<=1){
     if (matched>=0){throw new Error("Ambiguous notes at tick " + change.tick);}
     matched=j;
    }
   }
   if (matched<0) {throw new Error("No existing note at tick " + change.tick);}
   var existing=group.getNote(matched);
   if (existing.getPitch()!==change.expectedPitch){
    throw new Error("Pitch changed since preview at tick " + change.tick);
   }
   if (typeof change.newPitch!=="undefined" && (change.newPitch<0||change.newPitch>127)){
    throw new Error("Invalid pitch at tick " + change.tick);
   }
   if (typeof change.newLyric==="string" && !change.newLyric.length){
    throw new Error("Empty lyric at tick " + change.tick);
   }
   for (j=0;j<indexes.length;j++)if(indexes[j]===matched){
    throw new Error("Duplicate edit at tick " + change.tick);
   }
   indexes.push(matched);
  }
  SV.getProject().newUndoRecord();
  for (i=0;i<payload.changes.length;i++){
   var change=payload.changes[i];
   var note=group.getNote(indexes[i]);
   if(typeof change.newPitch!=="undefined")note.setPitch(change.newPitch);
   if(typeof change.newLyric!=="undefined")note.setLyrics(change.newLyric);
  }
  SV.showMessageBox("cusic","Updated " + payload.changes.length +
    " EXISTING notes. Original note objects and parameter curves remain. Save As to a NEW SVP.");
 }catch(e){
  SV.showMessageBox("cusic safe edit error",String(e));
 }finally{SV.finish();}
}
