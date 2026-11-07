# cusic

**Cubase + music.** A local-first, headless music arrangement harness for Claude Code, Codex and humans. No cusic GUI and no Git embedded in the music engine.

> **Status: early MVP.** The Go CLI and SQLite revision engine are implemented; Standard MIDI File note import/export and a TypeScript/Zod Synthesizer V 2 Pro script generator are included. Direct Cubase control, Suno stem separation/analysis, native `.tg` parsing, VSQX/Piapro V4X output, DAWproject and automatic two-way DAW synchronization are **not yet implemented**.

## Architecture

- **Go CLI:** `cusic` (no daemon or web UI).
- **SQLite project:** `.cusic/project.db`, with per-edit revisions and note/track change records; no Git inside.
- **Score:** MIDI notes at **480 PPQ**, tempo and a single time signature per project.
- **SynthV 2 Pro:** TypeScript source, Zod validation on the host, compiled standalone ES5.1 JavaScript executed inside Synthesizer V. No Node/Zod required in SynthV.
- **Cubase:** import newly exported MIDI tracks; the app never modifies `.cpr` files.
- **TuxGuitar:** use its *Export → MIDI* feature first. Source files are preserved.
- **Suno:** use existing exported MIDI/stems as reference; direct audio conversion is future work.

## Setup

Requires Go 1.23+, Node.js 22+ and npm (Node is needed only for the SynthV script builder).

~~~bash
git clone https://github.com/ichiken26/cusic.git
cd cusic
go build -o ./bin/cusic ./cmd/cusic
./bin/cusic help
~~~

GitHub Actions also builds downloadable Windows, macOS and Linux CLI binaries and a compiled SynthV script artifact for this branch. Find them in the successful workflow run's **Artifacts** section.

Install in PATH as desired. The examples below assume `cusic` is on PATH, or use the built `./bin/cusic` executable. By default commands search the current directory and parents for `.cusic/project.db`; set `CUSIC_PROJECT=/path/to/project` to override.

## Start a song

~~~bash
mkdir -p ~/Music/my-song && cd ~/Music/my-song
cusic init --name "Demo" --tempo 168 --meter 4/4
cusic track add --name "Bass" --kind instrument
cusic track add --name "Teto" --kind vocal --singer teto_sv2
cusic note add --track Bass --tick 0 --duration 480 --pitch 40 --velocity 100
cusic status
cusic inspect --track Bass --bars 1:8
~~~

MIDI pitch 40 = E2. One quarter note is 480 ticks; in 4/4 each bar contains 1920 ticks. `inspect` and `status` emit machine-readable JSON.

### Import a MIDI file

~~~bash
cusic import /path/to/tuxguitar-export.mid
cusic track list
cusic export --format midi --out ./exports/arrangement.mid
~~~

The importer accepts SMF format 0/1 PPQ files (multichannel notes supported). It splits channels into tracks, preserves note pitch/velocity/timing and reads an initial tempo/time signature. MIDI with tempo or meter changes, SMPTE timing, and missing note-offs is rejected. It does **not** retain CCs, pitch bends, text lyrics, program changes or other expressive events. If tracks already exist, a MIDI tempo/meter must match the project. This is intentionally conservative.

### Edit a bounded interval safely

Get the current revision ID via `cusic status`. Copy [examples/patch.json](examples/patch.json), update `baseRevision`, the track and note data, then apply:

~~~bash
cusic patch apply ./patch.json
cusic diff 3
cusic log -n 10
cusic validate
cusic revert 3  # only if revision 3 is HEAD
~~~

The patch **replaces all notes** in the inclusive bar range on **one track**. The DB transaction saves the changes and an inverse revision. Stale `baseRevision` is rejected; notes crossing the edit boundary are rejected. Revert is restricted to HEAD for safety; reverting creates a new revision that can itself be reverted. `diff REV` shows the change set stored for that revision (not a Git diff). Remember: a patch deletes any notes in the target range not included in `notes`.

### Generate a Teto SV2 import script

1. Create a *vocal* track named exactly like the track used in Synthesizer V Studio 2 Pro.
2. Export a bounded payload from that vocal track.
3. Compile the TypeScript bridge and use the Zod-backed script generator.
4. Open the corresponding track's **main group** in SynthV and run the generated script from the *Scripts* menu. Save your SynthV project separately.

~~~bash
cusic export --format synthv --track Teto --bars 1:8 --out teto.json
cd /path/to/cusic/synthv
npm install
npm run build
node dist/host/prepare.js /absolute/path/to/teto.json --out /absolute/path/to/cusic-teto.js
~~~

In SynthV choose **Scripts → Open Scripts Folder**, place `cusic-teto.js` there, rescan scripts, select the Teto track and run **cusic: Apply Vocal Notes**. It checks the current track name and range, then replaces only notes fully inside the range. Notes crossing boundaries cause an error. **Back up the SynthV project before running**: the replaced notes' tuning/retakes are not retained. Scripts execute inside SynthV on the user's action; this is not remote GUI control.

This approach meets the TypeScript + Zod requirement while respecting SynthV 2's ES5.1 scripting runtime. The distributed script is an ES5.1 **build artifact**, not TypeScript or Node source.

## Reliability, security and limitations

- SQLite handles transactional updates. A revision is an application-level record, not a Git commit. `revert` only works on the latest revision.
- Exports refuse to overwrite existing files; choose a new output path.
- Existing Cubase or SynthV edits are **not automatically mirrored** into SQLite. Export and re-import via new MIDI tracks when needed; avoid overwriting source data.
- No audio DSP, AI inference or Suno-to-MIDI transcription is bundled. Claude Code/Codex decide the musical notes, then call cusic.
- MusicXML, native TuxGuitar, DAWproject, VSQX, full MIDI expressive events, advanced tempo maps and branching histories remain future adapters.
- MIDI import's initial tempo/meter is included in its revision, so reverting an initial import also restores the project settings.
- Backup a live SQLite project using SQLite's backup mechanism (not a blind copy of only the DB file during WAL writes). The `.cusic` directory is gitignored.

## Development

~~~bash
go test -mod=mod ./...
go vet -mod=mod ./...
cd synthv && npm install && npm run build && npm test
~~~

See [AGENTS.md](AGENTS.md) for model-independent agent instructions and [CLAUDE.md](CLAUDE.md) for Claude Code.
