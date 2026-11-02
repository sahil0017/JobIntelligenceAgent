FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt \
    && playwright install --with-deps chromium
COPY cortex ./cortex
ENV CORTEX_DATA_DIR=/data PYTHONUNBUFFERED=1
# "::" listens on IPv6 as well, which Railway private networking requires
CMD ["sh", "-c", "mkdir -p $CORTEX_DATA_DIR && exec python -m uvicorn cortex.server:app --host :: --port 8890"]
