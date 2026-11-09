"""cusic's offline NumPy/SciPy audio analysis worker.

Reads a JSON request from stdin and writes one JSON response to stdout.
Never modifies input WAV files. Large WAVs are sampled at evenly spaced
windows, so metrics are estimates, NOT integrated LUFS or auditory masking.
"""
from __future__ import annotations

import json
import math
import os
import sys

import numpy as np
from scipy.io import wavfile

BANDS = (
    ("sub_20_60", 20, 60),
    ("bass_60_120", 60, 120),
    ("bass_120_250", 120, 250),
    ("lowmid_250_500", 250, 500),
    ("mid_500_2000", 500, 2000),
    ("presence_2000_6000", 2000, 6000),
    ("high_6000_16000", 6000, 16000),
    ("air_16000_20000", 16000, 20000),
)
MAX_FILES = 32
MAX_WINDOWS = 2048
FFT_SIZE = 8192


def _to_float64(frames: np.ndarray) -> np.ndarray:
    """Convert PCM and floating WAV samples to nominal full-scale [-1, 1]."""
    if frames.dtype.kind == "f":
        return frames.astype(np.float64, copy=False)
    if frames.dtype.kind == "i":
        return frames.astype(np.float64) / float(2 ** (frames.dtype.itemsize * 8 - 1))
    if frames.dtype.kind == "u" and frames.dtype.itemsize == 1:
        return (frames.astype(np.float64) - 128.0) / 128.0
    raise ValueError(f"unsupported WAV dtype {frames.dtype}")


def _db(amplitude: float) -> float:
    return round(20 * math.log10(max(1e-12, float(amplitude))), 2)


def analyze_file(name: str, path: str, max_windows: int = 512) -> tuple[dict, np.ndarray]:
    if not name or not isinstance(name, str):
        raise ValueError("audio name required")
    if not isinstance(path, str) or not os.path.isfile(path):
        raise ValueError(f"not an existing WAV file: {path}")
    if os.path.getsize(path) > 16 * 1024**3:
        raise ValueError("WAV exceeds 16 GiB safety limit")
    try:
        sample_rate, data = wavfile.read(path, mmap=True)
    except ValueError as exc:
        # SciPy cannot memory-map 24-bit PCM. For those files, allow a
        # bounded in-memory decode rather than accidentally exhausting RAM.
        if os.path.getsize(path) > 256 * 1024**2:
            raise ValueError("24-bit/unmappable WAV exceeds 256 MiB; export 32-bit float WAV") from exc
        sample_rate, data = wavfile.read(path, mmap=False)
    if sample_rate < 8000 or sample_rate > 384000:
        raise ValueError("unsupported sample rate")
    if data.ndim == 1:
        channels = 1
    elif data.ndim == 2 and 1 <= data.shape[1] <= 32:
        channels = int(data.shape[1])
    else:
        raise ValueError("unsupported channel layout")
    total_samples = int(data.shape[0])
    if total_samples == 0:
        raise ValueError("empty WAV")
    if not (1 <= max_windows <= MAX_WINDOWS):
        raise ValueError(f"max_windows must be 1..{MAX_WINDOWS}")

    fft_size = min(FFT_SIZE, total_samples)
    hops = max(1, (total_samples - fft_size) // max(1, fft_size // 2) + 1)
    count = min(max_windows, hops)
    positions = np.linspace(0, total_samples - fft_size, count, dtype=np.int64)
    taper = np.hanning(fft_size).astype(np.float64)
    energy_normalizer = fft_size * max(np.sum(taper**2), 1e-20)
    frequencies = np.fft.rfftfreq(fft_size, d=1.0 / sample_rate)
    spectral_power = np.zeros(frequencies.shape[0], dtype=np.float64)
    mean_sq = 0.0
    peak = 0.0
    stereo_x = stereo_y = stereo_x2 = stereo_y2 = stereo_xy = 0.0
    for pos in positions:
        frame = _to_float64(np.asarray(data[int(pos):int(pos) + fft_size]))
        if not np.all(np.isfinite(frame)):
            raise ValueError("WAV contains NaN/Inf samples")
        peak = max(peak, float(np.max(np.abs(frame))))
        mean_sq += float(np.mean(frame**2))
        if channels == 1:
            mono = frame
        else:
            mono = np.mean(frame, axis=1)
            left = frame[:, 0]
            right = frame[:, 1]
            stereo_x += float(np.sum(left))
            stereo_y += float(np.sum(right))
            stereo_x2 += float(np.dot(left, left))
            stereo_y2 += float(np.dot(right, right))
            stereo_xy += float(np.dot(left, right))
        transformed = np.fft.rfft(mono * taper)
        spectral_power += np.abs(transformed)**2

    spectral_power /= count
    one_sided = np.full(spectral_power.shape, 2.0, dtype=np.float64)
    one_sided[0] = 1.0
    if fft_size % 2 == 0:
        one_sided[-1] = 1.0
    bin_energy = spectral_power * one_sided / energy_normalizer
    band_rms_dbfs = {}
    for label, lower, upper in BANDS:
        selected = (frequencies >= lower) & (frequencies < upper)
        band_rms_dbfs[label] = _db(math.sqrt(float(np.sum(bin_energy[selected]))))

    usable = (frequencies >= 20) & (frequencies <= min(20000, sample_rate / 2))
    power = bin_energy[usable]
    freqs = frequencies[usable]
    total_power = float(np.sum(power))
    if total_power > 1e-20 and freqs.size:
        centroid = float(np.dot(freqs, power) / total_power)
        cumulative = np.cumsum(power)
        rolloff = float(freqs[min(int(np.searchsorted(cumulative, total_power * 0.95)), len(freqs) - 1)])
        top_idx = np.argsort(power)[-3:][::-1]
        dominant = [round(float(freqs[i]), 1) for i in top_idx if power[i] > total_power * 0.01]
    else:
        centroid, rolloff, dominant = 0.0, 0.0, []

    correlation = None
    if channels >= 2:
        count_samples = len(positions) * fft_size
        vx = stereo_x2 - stereo_x**2 / count_samples
        vy = stereo_y2 - stereo_y**2 / count_samples
        if vx > 1e-16 and vy > 1e-16:
            correlation = round(float(np.clip(
                (stereo_xy - stereo_x * stereo_y / count_samples) / math.sqrt(vx * vy), -1, 1
            )), 4)
    report = {
        "name": name, "sampleRate": int(sample_rate), "channels": channels,
        "durationSeconds": round(total_samples / sample_rate, 4),
        "windowsAnalyzed": count, "windowSamples": fft_size,
        "sampledPeakDbfs": _db(peak),
        "sampledRmsDbfs": _db(math.sqrt(mean_sq / count)),
        "bandRmsDbfs": band_rms_dbfs,
        "spectralCentroidHz": round(centroid, 1),
        "spectralRolloff95Hz": round(rolloff, 1),
        "dominantFrequenciesHz": dominant,
        "stereoCorrelation": correlation,
    }
    return report, bin_energy


def analyze_many(request: dict) -> dict:
    files = request.get("files", [])
    max_windows = request.get("maxWindows", 512)
    if not isinstance(files, list) or not (1 <= len(files) <= MAX_FILES):
        raise ValueError(f"files must contain 1..{MAX_FILES} WAV stems")
    if type(max_windows) is not int or not (1 <= max_windows <= MAX_WINDOWS):
        raise ValueError(f"maxWindows must be 1..{MAX_WINDOWS}")
    reports, spectra = [], []
    names = set()
    for item in files:
        if not isinstance(item, dict):
            raise ValueError("invalid stem entry")
        name, path = item.get("name"), item.get("path")
        if name in names:
            raise ValueError(f"duplicate stem name {name}")
        names.add(name)
        report, energy = analyze_file(name, path, max_windows)
        reports.append(report)
        # Band-based overlap survives differing sample rates and FFT sizes.
        spectra.append(np.array([10.0 ** (report["bandRmsDbfs"][key] / 20.0)
                                 for key, _, _ in BANDS], dtype=np.float64))
    pairs = []
    for i in range(len(reports)):
        for j in range(i + 1, len(reports)):
            a, b = spectra[i], spectra[j]
            denom = float(np.linalg.norm(a) * np.linalg.norm(b))
            similarity = float(np.dot(a, b) / denom) if denom > 1e-16 else 0.0
            pairs.append({
                "left": reports[i]["name"], "right": reports[j]["name"],
                "bandSpectralSimilarity": round(float(np.clip(similarity, 0.0, 1.0)), 4)
            })
    pairs.sort(key=lambda p: -p["bandSpectralSimilarity"])
    return {
        "version": 1, "engine": "numpy-scipy",
        "method": "sampled Hann-window rFFT, energy-integrated frequency bands",
        "stems": reports, "pairs": pairs,
        "limitations": "Estimated from sampled windows: NOT LUFS, true peak, phase alignment, causal masking, or an automatic EQ recommendation. High similarity is not proof of audible masking."
    }


def main() -> int:
    try:
        request = json.load(sys.stdin)
        result = analyze_many(request)
        json.dump(result, sys.stdout, ensure_ascii=False, allow_nan=False, separators=(",", ":"))
        sys.stdout.write("\n")
        return 0
    except (OSError, ValueError, TypeError) as exc:
        print(f"cusic audio worker: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
