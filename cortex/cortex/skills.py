"""Skill extraction against a curated vocabulary with aliases."""

from __future__ import annotations

import re

# canonical name -> alias patterns. Patterns are matched on word boundaries, case-insensitively
# unless the alias is a common English word (e.g. "Go"), which must match its exact casing.
_VOCAB: dict[str, list[str]] = {
    "Go": [r"(?-i:\bGo\b)", r"golang"],
    "Python": [r"python3?"],
    "Java": [r"java(?!\s*script)"],
    "JavaScript": [r"javascript", r"\bjs\b", r"ecmascript"],
    "TypeScript": [r"typescript", r"\bts\b"],
    "Kotlin": [r"kotlin"],
    "Scala": [r"scala"],
    "Rust": [r"rust"],
    "C++": [r"c\+\+"],
    "C#": [r"c#", r"\.net", r"dotnet"],
    "Ruby": [r"ruby"],
    "PHP": [r"php"],
    "Swift": [r"swift"],
    "SQL": [r"\bsql\b"],
    "React": [r"react(?:\.?js)?"],
    "Node.js": [r"node(?:\.?js)?"],
    "Angular": [r"angular"],
    "Vue": [r"vue(?:\.?js)?"],
    "Spring Boot": [r"spring\s*boot", r"\bspring\b"],
    "Django": [r"django"],
    "FastAPI": [r"fastapi"],
    "Flask": [r"flask"],
    "Microservices": [r"microservices?"],
    "REST APIs": [r"rest(?:ful)?\s*apis?", r"\brest\b"],
    "GraphQL": [r"graphql"],
    "gRPC": [r"grpc"],
    "Kafka": [r"kafka"],
    "RabbitMQ": [r"rabbitmq"],
    "PostgreSQL": [r"postgres(?:ql)?"],
    "MySQL": [r"mysql"],
    "MongoDB": [r"mongo(?:db)?"],
    "Redis": [r"redis"],
    "Elasticsearch": [r"elasticsearch", r"opensearch"],
    "AWS": [r"\baws\b", r"amazon web services"],
    "GCP": [r"\bgcp\b", r"google cloud"],
    "Azure": [r"azure"],
    "Docker": [r"docker"],
    "Kubernetes": [r"kubernetes", r"\bk8s\b"],
    "Terraform": [r"terraform"],
    "CI/CD": [r"ci\s*/\s*cd", r"continuous (?:integration|delivery)", r"jenkins", r"github actions"],
    "Linux": [r"linux"],
    "Git": [r"\bgit\b"],
    "Machine Learning": [r"machine learning", r"\bml\b"],
    "Deep Learning": [r"deep learning", r"neural networks?"],
    "PyTorch": [r"pytorch"],
    "TensorFlow": [r"tensorflow"],
    "NLP": [r"\bnlp\b", r"natural language processing"],
    "LLMs": [r"\bllms?\b", r"large language models?", r"generative ai", r"genai"],
    "Data Engineering": [r"data engineering", r"\betl\b", r"airflow", r"spark"],
    "System Design": [r"system design", r"distributed systems?", r"scalab(?:le|ility)"],
    "Testing": [r"unit testing", r"integration testing", r"\btdd\b", r"test automation"],
    "Agile": [r"agile", r"scrum"],
    "Security": [r"application security", r"oauth", r"\bowasp\b"],
}

_COMPILED = {
    name: re.compile(r"(?<![\w+#])(?:" + "|".join(patterns) + r")(?![\w+#])", re.IGNORECASE)
    for name, patterns in _VOCAB.items()
}


def extract_skills(text: str) -> list[str]:
    """Return canonical skills mentioned in `text`, in vocabulary order."""
    if not text:
        return []
    return [name for name, pattern in _COMPILED.items() if pattern.search(text)]
