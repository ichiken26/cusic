import { z } from "zod";

export const noteSchema = z.object({
  id: z.string().min(1),
  trackId: z.string().min(1),
  tick: z.number().int().nonnegative().safe(),
  duration: z.number().int().positive().safe(),
  pitch: z.number().int().min(0).max(127),
  velocity: z.number().int().min(1).max(127),
  lyric: z.string().max(4096)
}).strict();

export const payloadSchema = z.object({
  version: z.literal(1),
  trackId: z.string().min(1),
  trackName: z.string().min(1),
  ppq: z.number().int().positive(),
  fromTick: z.number().int().nonnegative().safe(),
  toTick: z.number().int().positive().safe(),
  notes: z.array(noteSchema).max(100000)
}).strict().superRefine((payload, ctx) => {
  if (payload.toTick <= payload.fromTick) {
    ctx.addIssue({ code: "custom", message: "toTick must exceed fromTick" });
  }
  const seen = new Set<string>();
  for (const [i, n] of payload.notes.entries()) {
    if (seen.has(n.id)) {
      ctx.addIssue({ code: "custom", path: ["notes", i], message: "Duplicate note id" });
    }
    seen.add(n.id);
    if (n.trackId !== payload.trackId ||
        n.tick < payload.fromTick ||
        n.tick + n.duration > payload.toTick) {
      ctx.addIssue({ code: "custom", path: ["notes", i], message: "Note outside target range or track" });
    }
  }
});
export type Payload = z.infer<typeof payloadSchema>;
