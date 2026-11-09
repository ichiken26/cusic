"""Offline, deterministic smoke tests for BPM proposal worker."""
import importlib.util
import pathlib
import tempfile
import unittest

import numpy as np
from scipy.io import wavfile

SCRIPT = pathlib.Path(__file__).resolve().parents[2] / "internal" / "tempo" / "worker.py"
SPEC = importlib.util.spec_from_file_location("cusic_tempo_worker", SCRIPT)
tempo_worker = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(tempo_worker)


class TempoAnalysisTests(unittest.TestCase):
    def test_percussive_pulse_train(self):
        sr, bpm, seconds = 8000, 120, 34
        audio = np.zeros(sr * seconds, dtype=np.float32)
        rng = np.random.default_rng(7)
        for n, start in enumerate(np.arange(0.25, seconds - 0.25, 60 / bpm)):
            idx = int(start * sr)
            length = 500
            hit = rng.standard_normal(length).astype(np.float32)
            hit *= np.exp(-np.arange(length) / 105).astype(np.float32)
            audio[idx:idx + length] += hit * (0.8 if n % 4 == 0 else 0.4)
        with tempfile.TemporaryDirectory() as directory:
            filename = pathlib.Path(directory) / "drums.wav"
            wavfile.write(filename, sr, audio)
            result = tempo_worker.analyze(str(filename))
        self.assertEqual(result["version"], 1)
        self.assertGreater(len(result["tempoCandidates"]), 0)
        self.assertGreater(len(result["beatTimesSeconds"]), 15)
        self.assertTrue(any(abs(c["bpm"] - bpm) < 3 or abs(c["bpm"] - bpm * 2) < 3
                            for c in result["tempoCandidates"]))

    def test_silent_audio_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            filename = pathlib.Path(directory) / "silence.wav"
            wavfile.write(filename, 8000, np.zeros(8000 * 9, dtype=np.float32))
            with self.assertRaises(ValueError):
                tempo_worker.analyze(str(filename))


if __name__ == "__main__":
    unittest.main()
