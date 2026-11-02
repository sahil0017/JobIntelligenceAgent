"""Semantic memory: the candidate's résumé indexed in ChromaDB for retrieval-augmented drafting.

Embeddings come from the configured Embedder (so the deep-learning engine applies here too) and
are handed to Chroma explicitly. The store is in-memory unless CORTEX_DATA_DIR is set.
"""

from __future__ import annotations

import hashlib
import os
import re
from dataclasses import dataclass

import chromadb

from .embeddings import Embedder

CHUNK_TARGET = 280
MAX_CHUNKS = 120


@dataclass(frozen=True)
class Evidence:
    text: str
    score: int


_CONTACT = re.compile(r"@|https?://|www\.|linkedin\.com|github\.com|\+?\d[\d\s().-]{8,}\d", re.I)


def _pieces(text: str) -> list[str]:
    """Break on newlines and bullet glyphs (PDF text often has no line breaks), then on sentence ends."""
    pieces: list[str] = []
    for part in re.split(r"[\n\r]+|\s[•·▪●]\s", text):
        part = re.sub(r"^[\s\-*•·▪●]+", "", part).strip()
        if len(part) > CHUNK_TARGET * 2:
            pieces.extend(sentence.strip() for sentence in re.split(r"(?<=[.;])\s+(?=[A-Z])", part))
        elif part:
            pieces.append(part)
    return pieces


def chunk_text(text: str) -> list[str]:
    """Split a résumé into bullet-sized passages, merging fragments that are too short to mean much
    and leaving out contact details so they never end up in a drafted bullet."""
    chunks: list[str] = []
    buffer = ""
    for piece in _pieces(text):
        if _CONTACT.search(piece):
            continue
        buffer = f"{buffer} {piece}".strip() if buffer and len(buffer) < 60 else piece
        if len(buffer) >= 60:
            chunks.append(buffer[: CHUNK_TARGET * 2])
            buffer = ""
    if buffer:
        chunks.append(buffer)
    return chunks[:MAX_CHUNKS]


class Memory:
    def __init__(self, embedder: Embedder, data_dir: str | None = None):
        self.embedder = embedder
        data_dir = data_dir if data_dir is not None else os.environ.get("CORTEX_DATA_DIR", "")
        self._client = chromadb.PersistentClient(path=data_dir) if data_dir else chromadb.EphemeralClient()

    def _collection_name(self, chunks: list[str]) -> str:
        digest = hashlib.sha256("\n".join(chunks).encode()).hexdigest()[:20]
        engine = hashlib.sha256(self.embedder.name.encode()).hexdigest()[:6]
        return f"resume-{engine}-{digest}"

    def index(self, chunks: list[str]) -> str:
        """Index chunks (idempotent per content) and return the collection name."""
        name = self._collection_name(chunks)
        collection = self._client.get_or_create_collection(name, metadata={"hnsw:space": "cosine"})
        if collection.count() != len(chunks):
            vectors = self.embedder.embed(chunks)
            collection.upsert(
                ids=[f"c{i}" for i in range(len(chunks))],
                documents=chunks,
                embeddings=vectors.tolist(),
            )
        return name

    def search(self, name: str, query: str, k: int = 5) -> list[Evidence]:
        collection = self._client.get_collection(name)
        count = collection.count()
        if count == 0:
            return []
        vector = self.embedder.embed([query])[0].tolist()
        found = collection.query(query_embeddings=[vector], n_results=min(k, count))
        documents = found["documents"][0]
        distances = found["distances"][0]
        return [Evidence(text=doc, score=self.embedder.calibrate(1.0 - float(dist))) for doc, dist in zip(documents, distances)]
