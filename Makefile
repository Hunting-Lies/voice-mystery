.PHONY: run test demo tidy asr-deps

run:
	@set -a; [ -f .env ] && . ./.env; set +a; \
	go run ./cmd/server -addr :8080 -web ./web

# 本地按住说话 ASR（需 ffmpeg；Python 建议 3.11–3.13）
asr-deps:
	@command -v ffmpeg >/dev/null || (echo "请先安装 ffmpeg: brew install ffmpeg" && exit 1)
	@PY=$${ASR_VENV_PYTHON:-python3.13}; \
	command -v $$PY >/dev/null || PY=python3; \
	$$PY -m venv .venv-asr && .venv-asr/bin/pip install -U pip && \
	.venv-asr/bin/pip install -r scripts/requirements-asr.txt

test:
	go test ./...

tidy:
	go mod tidy

demo:
	bash ./scripts/demo.sh
