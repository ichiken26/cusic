# cusic agent instructions

cusic is a local-first music score editor. The agent must use the CLI rather than
editing the SQLite database directly. The database is an implementation detail.

1. Begin by calling `cusic status`, `cusic track list`, and `cusic inspect --track ...`.
2. Preserve the user's DAW edits. Import an exported MIDI file into a new track,
   never assume that cusic knows unpublished edits in Cubase or SynthV.
3. For musical edits generate a JSON patch based on the current `head` revision.
   Use `cusic patch apply path.json`. Each patch must name exactly one track and
   a closed, 1-based bar interval. All note onsets use 480 PPQ absolute ticks.
4. A patch replaces the entire specified interval. Always include notes to retain
   inside the interval. Notes spanning its edges cause a safe failure.
5. Do not mutate unrelated tracks. Examine `cusic diff REV` and
   `cusic validate` after each change.
6. Export MIDI to a NEW file. Do not overwrite a user's Cubase project or source
   audio. For SynthV generate a bounded payload and run Zod validation before
   importing it into the editor.
7. On error, don't retry destructive edits blindly. Reinspect HEAD and generate a
   fresh patch. `cusic revert REV` reverses HEAD only.
8. Do not claim Suno audio stems, arbitrary TuxGuitar native files, VSQX, or
   Cubase projects are parsed; those adapters are not implemented yet.
9. The project keeps musical revisions in SQLite, not Git. Back up
   `.cusic/project.db` using SQLite's backup tools when needed.

Example patch file: examples/patch.json. Commands: `cusic help`.
