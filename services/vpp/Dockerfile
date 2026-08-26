FROM python:3.11-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Copy pyproject.toml and source
COPY pyproject.toml README.md /app/
COPY vpp /app/vpp/

# Install python package and dependencies
RUN pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir hatchling && \
    pip install --no-cache-dir -e .

EXPOSE 8000

ENV PORT=8000
ENV PYTHONUNBUFFERED=1

CMD ["uvicorn", "vpp.api.app:app", "--host", "0.0.0.0", "--port", "8000"]
