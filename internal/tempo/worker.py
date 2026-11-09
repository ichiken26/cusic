"""Read-only drum onset / tempo estimator for cusic.

Estimate only: neither a precise transcription nor a guarantee of a correct
beat-grid phase. In particular, half/double-time and syncopation are ambiguous.
"""
import json
import math
import sys

import numpy as np
from scipy import signal
from scipy.io import wavfile


def analyze(path):
    sr, data = wavfile.read(path, mmap=False)
    if sr < 8000 or sr > 192000 or data.ndim not in (1, 2):
        raise ValueError("unsupported WAV sample rate/channels")
    if len(data) < sr * 8:
        raise ValueError("at least 8 seconds of audio are required for tempo analysis")
    duration = len(data) / sr
    # Limit inference cost, without modifying the file. Use regularly spaced
    # sections across the track so a long song is not judged only by its intro.
    if duration > 360:
        raise ValueError("tempo analysis supports WAVs up to 6 minutes")
    if data.ndim == 2:
        x = data.astype(np.float32).mean(axis=1)
    else:
        x = data.astype(np.float32)
    if data.dtype.kind in "iu":
        scale = float(2 ** (8 * data.dtype.itemsize - 1))
        x = x / scale
    if not np.all(np.isfinite(x)):
        raise ValueError("non-finite audio samples")
    if float(np.max(np.abs(x))) < 0.00001:
        raise ValueError("silent WAV: unable to infer tempo")
    # Resample to 8000Hz with anti-aliasing; preserve audio timeline.
    target_sr = 8000
    gcd = math.gcd(int(sr), target_sr)
    x = signal.resample_poly(x, target_sr // gcd, int(sr) // gcd).astype(np.float32)
    x -= float(np.mean(x))
    hop, nfft = 256, 1024
    _, _, z = signal.stft(x, target_sr, nperseg=nfft, noverlap=nfft - hop,
                          boundary=None, padded=False)
    magnitude = np.abs(z).astype(np.float32)
    # Percussive novelty: positive spectral differences. Normalize to prevent
    # one extremely loud hit from dominating the autocorrelation.
    novelty = np.maximum(np.diff(magnitude, axis=1), 0).sum(axis=0)
    novelty = np.pad(novelty, (1, 0))
    novelty = signal.convolve(novelty, np.ones(3, dtype=np.float32) / 3,
                              mode="same")
    novelty = np.maximum(novelty - np.percentile(novelty, 50), 0)
    if np.max(novelty) <= 1e-8:
        raise ValueError("no detectable rhythmic attacks")
    novelty = np.minimum(novelty, np.percentile(novelty, 95) * 3 + 1e-8)
    novelty = novelty / (np.max(novelty) + 1e-9)
    frame_rate = target_sr / hop
    # Evaluate whole-song and local 20-40s correlations. We intentionally keep
    # multiple candidates visible: this algorithm cannot distinguish 90/180
    # BPM reliably from every drum arrangement.
    bpms = np.arange(60.0, 241.0, 0.5)
    frames = len(novelty)
    def score_segment(y):
        y = y - np.mean(y)
        corr = signal.correlate(y, y, method="fft", mode="full")[len(y)-1:]
        corr = corr / max(float(corr[0]), 1e-9)
        lags = np.arange(len(corr), dtype=np.float64)
        scores = np.zeros_like(bpms)
        for multiplier, weight in ((1, 1.0), (2, 0.45), (3, 0.15)):
            lag = 60.0 * frame_rate * multiplier / bpms
            scores += weight * np.interp(lag, lags, corr, left=0.0, right=0.0)
        return scores
    whole = score_segment(novelty)
    segments = []
    win = int(30 * frame_rate)
    if frames >= win:
        for start in range(0, frames - win + 1, max(win // 2, 1)):
            segments.append(score_segment(novelty[start:start+win]))
    combined = 0.6 * whole + 0.4 * np.mean(segments, axis=0) if segments else whole
    order = np.argsort(combined)[::-1]
    picked = []
    for i in order:
        bpm = float(bpms[i])
        if all(abs(bpm - candidate[0]) > 3 for candidate in picked):
            picked.append((bpm, float(combined[i])))
        if len(picked) >= 6:
            break
    best_bpm = picked[0][0]
    # Strong/weak are indicators only; do not silently accept the result.
    best_score = max(picked[0][1], 0.0)
    confidence = float(np.clip(best_score * 2.2, 0, 1))
    local_bpms = []
    for s in segments:
        j = int(np.argmax(s))
        local_bpms.append(float(bpms[j]))
    # Estimate beat anchors using a fixed-tempo grid. These anchors are
    # exploratory; tempo rendering defaults to uniform mode and requires a
    # separate explicit confirmation before changing any waveform.
    period = 60 / best_bpm
    offsets = np.arange(0, period, 1 / frame_rate)
    frame_times = np.arange(frames) / frame_rate + nfft / (2 * target_sr)
    phases = []
    for phase in offsets:
        targets = np.arange(phase, duration - 0.02, period)
        idx = np.searchsorted(frame_times, targets)
        idx = np.clip(idx, 0, frames-1)
        phases.append(float(np.sum(novelty[idx])))
    phase = float(offsets[int(np.argmax(phases))])
    beat_times = np.arange(phase, duration, period)
    anchors = []
    for t in beat_times:
        j = int(np.argmin(np.abs(frame_times - t)))
        reach = max(1, int(0.075 * frame_rate))
        left, right = max(0, j-reach), min(frames, j+reach+1)
        if left >= right:
            continue
        peak = left + int(np.argmax(novelty[left:right]))
        if float(novelty[peak]) > 0.13 and abs(frame_times[peak] - t) < 0.075:
            anchors.append(round(float(frame_times[peak]), 6))
        else:
            anchors.append(round(float(t), 6))
    # Adjacent attack corrections must be monotonic, or later stretching
    # could fold the timeline.
    anchors = [v for i, v in enumerate(anchors) if i == 0 or v > anchors[i-1]]
    stable_local = bool(local_bpms) and all(
        min(abs(x - best_bpm), abs(x * 2 - best_bpm),
            abs(x / 2 - best_bpm)) <= 3 for x in local_bpms
    )
    return {
        "version": 1, "engine": "numpy-scipy-onset",
        "estimatedBpm": best_bpm,
        "confidence": round(confidence, 3),
        "tempoCandidates": [{"bpm": round(b, 2), "score": round(s, 4)}
                            for b, s in picked],
        "localBpm": local_bpms,
        "stableLocalTempo": stable_local,
        "durationSeconds": duration,
        "beatTimesSeconds": anchors,
        "limitations": "Approximate beat/onset inference; half/double tempo, "
                       "syncopation and beat phase may be wrong. Listen and "
                       "confirm before accepting a tempo or warping stems."
    }


def main():
    req = json.load(sys.stdin)
    if not isinstance(req, dict) or not isinstance(req.get("path"), str):
        raise ValueError("expected JSON object with path")
    print(json.dumps(analyze(req["path"]), allow_nan=False))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(str(exc), file=sys.stderr)
        sys.exit(1)
