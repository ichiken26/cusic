/*
 * cusic Synthesizer V Studio 2 Pro bridge.
 * Developed in TypeScript. Release output: standalone ES5.1 JavaScript.
 * No Zod/Node dependencies are loaded inside the editor: the input has already
 * been validated by src/prepare.ts before embedding it here.
 */
type CusicNote = {
  id: string;
  trackId: string;
  tick: number;
  duration: number;
  pitch: number;
  velocity: number;
  lyric: string;
};
type CusicPayload = {
  version: number;
  trackId: string;
  trackName: string;
  ppq: number;
  fromTick: number;
  toTick: number;
  notes: CusicNote[];
};
function getClientInfo() {
  return {
    name: "cusic: Apply Vocal Notes",
    category: "cusic",
    author: "cusic contributors",
    versionNumber: 1,
    minEditorVersion: 65540
  };
}
function main() {
  try {
    var payload = JSON.parse("__CUSIC_PAYLOAD__") as CusicPayload;
    // This check also prevents running an unprepared template accidentally.
    if (!payload || payload.version !== 1 || payload.ppq <= 0) {
      throw new Error("This script has no valid embedded cusic payload.");
    }
    var editor = SV.getMainEditor();
    var track = editor.getCurrentTrack();
    if (!track || track.getName() !== payload.trackName) {
      throw new Error("Select the SynthV track named '" + payload.trackName + "' before running.");
    }
    var reference = editor.getCurrentGroup();
    if (!reference || !reference.isMain()) {
      throw new Error("Open the main note group of that track before running cusic.");
    }
    var group = reference.getTarget();
    var timeOffset = reference.getTimeOffset();
    var start = SV.quarter2Blick(payload.fromTick / payload.ppq) - timeOffset;
    var end = SV.quarter2Blick(payload.toTick / payload.ppq) - timeOffset;

    if (start < 0 || end <= start) {
      throw new Error("Target range is outside the current main group.");
    }
    // Validate *all* existing notes before mutating anything.
    var i: number;
    for (i = 0; i < group.getNumNotes(); ++i) {
      var old = group.getNote(i);
      if (old.getOnset() < end && old.getEnd() > start) {
        if (old.getOnset() < start || old.getEnd() > end) {
          throw new Error("An existing note crosses the selected boundary.");
        }
      }
    }
    // Bridge-side sanity checks in addition to Zod validation at build time.
    for (i = 0; i < payload.notes.length; ++i) {
      var n = payload.notes[i];
      if (n.trackId !== payload.trackId || n.duration <= 0 ||
          n.tick < payload.fromTick || n.tick + n.duration > payload.toTick ||
          n.pitch < 0 || n.pitch > 127) {
        throw new Error("Invalid embedded note #" + i);
      }
    }
    SV.getProject().newUndoRecord();
    // Reverse deletion preserves indices.
    for (i = group.getNumNotes() - 1; i >= 0; --i) {
      var existing = group.getNote(i);
      if (existing.getOnset() >= start && existing.getEnd() <= end) {
        group.removeNote(i);
      }
    }
    for (i = 0; i < payload.notes.length; ++i) {
      var item = payload.notes[i];
      var note = SV.create("Note");
      note.setTimeRange(
        SV.quarter2Blick(item.tick / payload.ppq) - timeOffset,
        SV.quarter2Blick(item.duration / payload.ppq)
      );
      note.setPitch(item.pitch);
      note.setLyrics(item.lyric || "la");
      group.addNote(note);
    }
    SV.showMessageBox("cusic", "Applied " + payload.notes.length +
      " notes to " + payload.trackName + ". Save the SynthV project when ready.");
  } catch (e) {
    SV.showMessageBox("cusic error", String(e));
  } finally {
    SV.finish();
  }
}
