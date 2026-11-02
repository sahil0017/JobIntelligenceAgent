#!/bin/bash
ollama serve &
server=$!
until ollama list >/dev/null 2>&1; do sleep 1; done
ollama pull "$OLLAMA_MODEL"
wait $server
