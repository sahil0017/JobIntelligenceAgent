#!/bin/bash
ollama serve &
server=$!
until ollama list >/dev/null 2>&1; do sleep 1; done
ollama pull "$OLLAMA_MODEL"
# The host reports ~48 threads but the container only gets a small CPU quota; oversubscribing
# makes generation orders of magnitude slower. Re-create the tag with a matching thread count.
printf 'FROM %s\nPARAMETER num_thread %s\n' "$OLLAMA_MODEL" "$OLLAMA_NUM_THREAD" > /tmp/Modelfile
ollama create "$OLLAMA_MODEL" -f /tmp/Modelfile
wait $server
