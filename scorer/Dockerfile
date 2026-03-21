# Dockerfile — container image for the scorer service
#
# Installs Python dependencies and runs the scorer package as a one-shot
# container via `python -m scorer`.

FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
CMD ["python", "-m", "scorer"]
