"""Offline deterministic tests for NumPy/SciPy spectral analysis."""
import importlib.util
import math
from pathlib import Path
import tempfile
import unittest

import numpy as np
from scipy.io import wavfile

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("cusic_worker", ROOT / "internal" / "analysis" / "worker.py")
worker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(worker)


class AnalyzeTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.sr = 48000
        self.t = np.arange(self.sr, dtype=np.float64) / self.sr

    def make_wav(self, name, frequencies, stereo=False):
        signal = sum(0.2 * np.sin(2 * np.pi * f * self.t) for f in frequencies)
        samples = np.stack((signal, signal), axis=-1) if stereo else signal
        dest = self.root / name
        wavfile.write(dest, self.sr, samples.astype(np.float32))
        return str(dest)

    def test_fft_band_analysis_and_pairwise_similarity(self):
        bass = self.make_wav("bass.wav", [80.0])
        kick = self.make_wav("kick.wav", [80.0])
        voice = self.make_wav("vocal.wav", [1800.0], stereo=True)
        result = worker.analyze_many({"files": [
            {"name": "bass", "path": bass}, {"name": "kick", "path": kick},
            {"name": "vocal", "path": voice}
        ], "maxWindows": 64})
        self.assertEqual(result["engine"], "numpy-scipy")
        b, _, v = result["stems"]
        self.assertEqual(b["sampleRate"], 48000)
        self.assertEqual(v["channels"], 2)
        self.assertAlmostEqual(v["stereoCorrelation"], 1, places=3)
        self.assertGreater(b["bandRmsDbfs"]["bass_60_120"],
                           b["bandRmsDbfs"]["presence_2000_6000"] + 40)
        self.assertGreater(v["bandRmsDbfs"]["mid_500_2000"],
                           v["bandRmsDbfs"]["bass_60_120"] + 35)
        self.assertAlmostEqual(b["spectralCentroidHz"], 80, delta=10)
        self.assertEqual(result["pairs"][0]["left"], "bass")
        self.assertEqual(result["pairs"][0]["right"], "kick")
        self.assertGreater(result["pairs"][0]["bandSpectralSimilarity"], 0.99)

    def test_validates_paths_and_empty_file_list(self):
        with self.assertRaisesRegex(ValueError, "1..32"):
            worker.analyze_many({"files": []})
        with self.assertRaisesRegex(ValueError, "existing WAV"):
            worker.analyze_many({"files": [{"name": "bad", "path": str(self.root / "missing.wav")} ]})
        with self.assertRaisesRegex(ValueError, "maxWindows"):
            worker.analyze_many({"files": [{"name": "no", "path": "none"}], "maxWindows": 0})

    def test_int16_pcm_and_stereo_antiphase(self):
        p = self.root / "stereo.wav"
        sig = (10000 * np.sin(2 * np.pi * 440 * self.t)).astype(np.int16)
        wavfile.write(p, self.sr, np.stack((sig, -sig), axis=-1))
        report, _ = worker.analyze_file("stereo", str(p), 32)
        self.assertEqual(report["channels"], 2)
        self.assertLess(report["stereoCorrelation"], -0.99)
        self.assertTrue(math.isfinite(report["sampledRmsDbfs"]))


if __name__ == "__main__":
    unittest.main()
