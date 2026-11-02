"""Text embedding engines.

`HashingEmbedder` is the dependency-light default: signed feature hashing over word and
character n-grams. `SentenceTransformerEmbedder` is the deep-learning engine: a pretrained
transformer that captures meaning ("Postgres" ~ "relational database"). Select it with
CORTEX_EMBEDDER=sentence-transformers after `pip install -r requirements-ml.txt`.
"""

from __future__ import annotations

import hashlib
import os
import re
from typing import Protocol, Sequence

import numpy as np

_WORD = re.compile(r"[a-z0-9+#.]+")


class Embedder(Protocol):
    name: str
    deep_learning: bool

    def embed(self, texts: Sequence[str]) -> np.ndarray:
        """Return one L2-normalised row per text."""

    def calibrate(self, cosine: float) -> int:
        """Map a cosine similarity to a 0-100 relevance score for this engine."""


class HashingEmbedder:
    name = "hashing-ngrams"
    deep_learning = False

    def __init__(self, dimensions: int = 1024):
        self.dimensions = dimensions

    def _features(self, text: str) -> list[str]:
        words = _WORD.findall(text.lower())
        features = list(words)
        features += [f"{a} {b}" for a, b in zip(words, words[1:])]
        for word in words:
            padded = f"^{word}$"
            features += [padded[i : i + 3] for i in range(len(padded) - 2)]
        return features

    def embed(self, texts: Sequence[str]) -> np.ndarray:
        matrix = np.zeros((len(texts), self.dimensions), dtype=np.float32)
        for row, text in enumerate(texts):
            for feature in self._features(text):
                digest = hashlib.blake2b(feature.encode(), digest_size=8).digest()
                value = int.from_bytes(digest, "big")
                matrix[row, value % self.dimensions] += 1.0 if (value >> 63) & 1 else -1.0
        matrix = np.sign(matrix) * np.sqrt(np.abs(matrix))
        norms = np.linalg.norm(matrix, axis=1, keepdims=True)
        return matrix / np.where(norms == 0, 1.0, norms)

    def calibrate(self, cosine: float) -> int:
        return int(round(max(0.0, min(1.0, (cosine - 0.15) / 0.40)) * 100))


class SentenceTransformerEmbedder:
    deep_learning = True

    def __init__(self, model_name: str = "sentence-transformers/all-MiniLM-L6-v2"):
        from sentence_transformers import SentenceTransformer  # optional heavy dependency

        self.name = model_name
        self._model = SentenceTransformer(model_name)

    def embed(self, texts: Sequence[str]) -> np.ndarray:
        return np.asarray(self._model.encode(list(texts), normalize_embeddings=True), dtype=np.float32)

    def calibrate(self, cosine: float) -> int:
        return int(round(max(0.0, min(1.0, (cosine - 0.1) / 0.6)) * 100))


class OnnxEmbedder:
    """The same all-MiniLM-L6-v2 transformer run on onnxruntime: a neural network without the torch install."""

    deep_learning = True

    def __init__(self) -> None:
        from chromadb.utils.embedding_functions import ONNXMiniLM_L6_V2  # ships with chromadb

        self.name = "onnx:all-MiniLM-L6-v2"
        self._model = ONNXMiniLM_L6_V2()
        self._model(["warm up"])

    def embed(self, texts: Sequence[str]) -> np.ndarray:
        vectors = np.asarray(self._model([t[:2000] for t in texts]), dtype=np.float32)
        norms = np.linalg.norm(vectors, axis=1, keepdims=True)
        return vectors / np.where(norms == 0, 1, norms)

    def calibrate(self, cosine: float) -> int:
        return int(round(max(0.0, min(1.0, (cosine - 0.1) / 0.6)) * 100))


def build_embedder(choice: str | None = None) -> Embedder:
    choice = (choice or os.environ.get("CORTEX_EMBEDDER", "auto")).strip().lower()
    if choice in ("auto", "onnx"):
        try:
            return OnnxEmbedder()
        except Exception:  # noqa: BLE001 - offline first run or missing runtime: stay usable
            if choice == "onnx":
                raise
            return HashingEmbedder()
    if choice in ("sentence-transformers", "sbert", "deep"):
        return SentenceTransformerEmbedder(os.environ.get("CORTEX_MODEL", "sentence-transformers/all-MiniLM-L6-v2"))
    if choice in ("hashing", ""):
        return HashingEmbedder()
    raise ValueError(f"unknown CORTEX_EMBEDDER {choice!r}; use 'hashing' or 'sentence-transformers'")
