# cusic agent instructions

cusic is a local-first music editing CLI, not an autonomous Cubase GUI driver.
Do not write the SQLite project database directly; use cusic commands.

1. First run `cusic status` and `cusic track list`. Inspect the target with `cusic inspect --track TRACK --bars START:END`. Track absolute onsets use 480 PPQ.
2. For a precise edit, construct a validated patch JSON with `baseRevision` equal to current HEAD, one track and an inclusive bar range. `cusic patch apply FILE.json` **replaces the whole range**, so include retained notes too. Changes crossing range boundaries fail safely.
3. Validate with `cusic diff REV` and `cusic validate`. Use `cusic revert REV` only if the revision is HEAD. Never blindly replay failed patch operations.
4. Treat Cubase and vocal synthesis as sources of human edits. **No live two-way synchronization exists.** Use `cusic sync plan --from file.dawproject` first. Only after reviewing the plan, use `cusic sync pull --from file.dawproject --base-revision N`. Vocal tracks are intentionally skipped to preserve their lyrics and tuning.
5. For TuxGuitar use `cusic import score.tg` only with TuxGuitar 2.x ZIP/XML. For TuxGuitar 1.x request exported MIDI. Do not claim TAB technique or pitch-bend retention.
6. For Suno, prefer already exported isolated WAVs/MIDI. `cusic suno import ./stems` stores WAV as immutable assets. Optional `cusic suno separate` and `cusic suno transcribe` invoke installed Demucs / Basic Pitch. Transcription is approximate. Do not claim audio-to-perfect-score conversion.
7. For Miku V4X, require a real Piapro Studio–exported VOCALOID4 `.vsqx` with the installed singer as a template; no voicebank IDs should be invented. Use `cusic export --format vsqx --template FILE --track Miku --out NEWFILE`. Lyrics should be kana one mora per note; check pronunciation by ear afterward.
8. For Teto SV2 use `cusic export --format synthv` followed by the TypeScript/Zod host preparer. Never run Node or Zod inside the SynthV ES5.1 scripting runtime. Actual application runs the compiled JS.
9. Export to fresh output files (MIDI, VSQX, DAWproject, SynthV); never modify a source `.cpr`, source audio or vocal project without consent and backups. Never claim that DAWproject or Piapro Studio import was validated on the user's machine.
10. SQLite holds revisions, not Git. Back up the DB with SQLite-safe backup tools, including WAL data.

Example musical patch: `examples/patch.json`.

## Agent-first workflow

Read `docs/agent-first.md` for project-scoped MCP setup and safe vocal/mix workflows. Use the MCP tools from `cusic mcp` rather than expecting users to know the CLI.
For an existing mixed song, use the **original Cubase-exported DAWproject** as the source of `mix_plan` / `mix_apply`, not a new cusic-generated session (audio region offsets may otherwise be lost).
Always ask approval before `mix_apply`; it creates a NEW .dawproject file. Never overwrite .cpr, source recordings, .svp, .vsqx or .ppsf.
Backup vocal source with `vocal_backup_source`. Use `vocal_vsqx_edit_existing` for safe existing VSQX note edits, or the TypeScript/Zod `synthv/src/safe.ts` script for existing SynthV notes; NEVER use destructive note replacement on tuned vocals.

## NumPy/SciPy spectral worker
Prefer the read-only MCP mix_analyze_spectral tool for quantitative frequency-domain mixing research. It uses NumPy rFFT and SciPy WAV input. When Python dependencies are missing, report the installation command (py -m pip install -r audio/requirements.txt) and fall back to mix_analyze_stems for Go-only approximate statistics. The sampled FFT results are not integrated LUFS, true peak or definitive auditory masking. Never alter the recording during analysis.
