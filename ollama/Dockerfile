FROM ollama/ollama:latest
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
# [::] so Railway private networking (IPv6) can reach it
ENV OLLAMA_HOST=[::]:11434 OLLAMA_MODEL=qwen2.5:3b OLLAMA_KEEP_ALIVE=30m OLLAMA_NUM_THREAD=8 OLLAMA_NUM_PARALLEL=2
ENTRYPOINT ["/entrypoint.sh"]
