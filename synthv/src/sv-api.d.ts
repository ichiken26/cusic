// Minimal typings for the subset of the official SV2 scripting API used by cusic.
// This file is a compile-time declaration, not bundled into the runtime script.
interface SvNote {
  getOnset(): number;
  getEnd(): number;
  setTimeRange(onset: number, duration: number): void;
  setPitch(pitch: number): void;
  setLyrics(lyrics: string): void;
}
interface SvNoteGroup {
  getNumNotes(): number;
  getNote(index: number): SvNote;
  removeNote(index: number): void;
  addNote(note: SvNote): number;
}
interface SvGroupReference {
  getTarget(): SvNoteGroup;
  getTimeOffset(): number;
  isMain(): boolean;
}
interface SvTrack { getName(): string; }
interface SvEditor {
  getCurrentGroup(): SvGroupReference | undefined;
  getCurrentTrack(): SvTrack | undefined;
}
declare const SV: {
  getMainEditor(): SvEditor;
  getProject(): { newUndoRecord(): void };
  create(type: "Note"): SvNote;
  quarter2Blick(quarter: number): number;
  showMessageBox(title: string, message: string): void;
  finish(): void;
};
