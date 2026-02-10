# Dual RTX 3090 (24GB each) configuration with VRAM/RAM scheduling

This example shows how to run llama-swap on a dual RTX 3090 system (24GB VRAM each, 48GB total) while using the **VRAM/RAM scheduler hints**. It converts each `llama-server` command to a `docker run` call using the `ghcr.io/ggml-org/llama.cpp:full-cuda` image.

Key points:

- **GPU VRAM caps**: clamp the scheduler to 24GB per GPU.
- **Host RAM cap**: limit scheduler RAM accounting to 240GB system RAM for non-spill models.
- **Initial usage hints**: set `initialVramMB` to 95% of available VRAM (single GPU or dual GPU). For `glm-4.7-dual` and `deepseek-v3-dual`, also set `initialCpuMB: 245760` (240GB) so the scheduler understands the large RAM needs from the start.

> Note: Replace `/models` with the path where your GGUF files live. The container binds it to `/models`.
>
> Docker GPU selection: for `fitPolicy: evict_to_fit`, llama-swap assigns a single GPU and injects `CUDA_VISIBLE_DEVICES=<index>`. For `fitPolicy: spill`, llama-swap injects all detected GPUs (for example `CUDA_VISIBLE_DEVICES=0,1`). To pass this into Docker, use `-e CUDA_VISIBLE_DEVICES` with `--gpus all`. For explicit dual-GPU lanes, set `CUDA_VISIBLE_DEVICES=0,1` directly in the docker command.

## Example configuration

```yaml
healthCheckTimeout: 300
startPort: 5800
logLevel: info

# Scheduler caps for a dual RTX 3090 setup (24GB each = 48GB total)
gpuVramCapsMB: [24576, 24576]

# Clamp host RAM for non-spill models to 240GB
hostRamCapMB: 245760

models:
  # ---------------------------------------------------------
  # GPU 0: FAST INFERENCE LANE (GLM Flash & Tools)
  # ---------------------------------------------------------
  glm-flash-q4:
    name: "GLM-4.7 Flash Q4_K_XL"
    fitPolicy: evict_to_fit
    initialVramMB: 23347
    initialCpuMB: 0
    cmd: >
      docker run --rm --gpus all
        -e LLAMA_SET_ROWS=1
        -e CUDA_VISIBLE_DEVICES
        -v /models:/models
        -p ${PORT}:${PORT}
        ghcr.io/ggml-org/llama.cpp:full-cuda
        --host 0.0.0.0 --port ${PORT}
        -m /models/GLM-4.7-Flash-GGUF-Q4_K_XL/GLM-4.7-Flash-UD-Q4_K_XL.gguf
        -c 202752
        --fit on
        --flash-attn on
        --cache-type-k q4_0 --cache-type-v q4_0
        --batch-size 8192 --ubatch-size 2048
        --no-webui
        --jinja --temp 1.0 --top-p 0.95 --min-p 0.01
        --repeat-penalty 1.0
    ttl: 900

  glm-flash-q8:
    name: "GLM-4.7 Flash Q8_K_XL"
    fitPolicy: evict_to_fit
    initialVramMB: 23347
    initialCpuMB: 0
    cmd: >
      docker run --rm --gpus all
        -e LLAMA_SET_ROWS=1
        -e CUDA_VISIBLE_DEVICES
        -v /models:/models
        -p ${PORT}:${PORT}
        ghcr.io/ggml-org/llama.cpp:full-cuda
        --host 0.0.0.0 --port ${PORT}
        -m /models/GLM-4.7-Flash-GGUF-Q8_K_XL/GLM-4.7-Flash-UD-Q8_K_XL.gguf
        -c 202752
        --flash-attn on
        --fit on
        --cpu-moe
        --cache-type-k q4_0 --cache-type-v q4_0
        --batch-size 8192 --ubatch-size 2048
        --no-webui
        --jinja --temp 1.0 --top-p 0.95 --min-p 0.01
        --repeat-penalty 1.0
    ttl: 900

  # ---------------------------------------------------------
  # GPU 1: CODING LANE (Qwen Coder Variants)
  # ---------------------------------------------------------
  qwen-30b-gpu1:
    name: "Qwen3 Coder 30B-A3B-Instruct Q4_K_XL"
    fitPolicy: evict_to_fit
    initialVramMB: 23347
    initialCpuMB: 0
    cmd: >
      docker run --rm --gpus all
        -e LLAMA_SET_ROWS=1
        -e CUDA_VISIBLE_DEVICES
        -v /models:/models
        -p ${PORT}:${PORT}
        ghcr.io/ggml-org/llama.cpp:full-cuda
        --host 0.0.0.0 --port ${PORT}
        -m /models/Qwen3-Coder-30B-A3B-Instruct-GGUF-Q4_K_XL/Qwen3-Coder-30B-A3B-Instruct-UD-Q4_K_XL.gguf
        -c 192000
        --flash-attn on
        --fit on
        --cache-type-k q4_0 --cache-type-v q4_0
        --no-webui
        --jinja
        --temp 0.7 --min-p 0.0 --top-p 0.80 --top-k 20 --repeat-penalty 1.05
    ttl: 900

  qwen-next:
    name: "Qwen3-Coder Next MXFP4_MOE"
    fitPolicy: evict_to_fit
    initialVramMB: 23347
    initialCpuMB: 120
    cmd: >
      docker run --rm --gpus all
        -e LLAMA_SET_ROWS=1
        -e CUDA_VISIBLE_DEVICES
        -v /models:/models
        -p ${PORT}:${PORT}
        ghcr.io/ggml-org/llama.cpp:full-cuda
        --host 0.0.0.0 --port ${PORT}
        -m /models/Qwen3-Coder-Next-MXFP4_MOE/Qwen3-Coder-Next-MXFP4_MOE.gguf
        -c 131072
        --flash-attn on
        --fit on
        --cpu-moe
        --cache-type-k q4_0 --cache-type-v q4_0
        --no-webui
        --jinja --reasoning-format none
        --seed 3407 --temp 1.0 --top-p 0.95 --min-p 0.01 --top-k 40
    ttl: 900

  # ---------------------------------------------------------
  # DUAL GPU: HEAVY DUTY & REASONING
  # ---------------------------------------------------------
  glm-flash-q8-dual:
    name: "GLM-4.7 Flash Q8_K_XL"
    concurrencyLimit: 1
    fitPolicy: evict_to_fit
    initialVramMB: 46759
    initialCpuMB: 245760
    cmd: >
      docker run --rm --gpus all
        -e LLAMA_SET_ROWS=1
        -e CUDA_VISIBLE_DEVICES=0,1
        -v /models:/models
        -p ${PORT}:${PORT}
        ghcr.io/ggml-org/llama.cpp:full-cuda
        --host 0.0.0.0 --port ${PORT}
        -m /models/GLM-4.7-Flash-GGUF-Q8_K_XL/GLM-4.7-Flash-UD-Q8_K_XL.gguf
        -c 262144
        --fit on
        --flash-attn on
        --cache-type-k q4_0 --cache-type-v q4_0
        --batch-size 8192 --ubatch-size 2048
        --no-webui
        --jinja
        --seed 3407 --temp 1.0 --top-p 0.95 --min-p 0.01
    ttl: 900

  qwen-next-dual:
    name: "Qwen3 Coder Next MXFP4_MOE"
    concurrencyLimit: 1
    fitPolicy: evict_to_fit
    initialVramMB: 46759
    initialCpuMB: 0
    cmd: >
      docker run --rm --gpus all
        -e LLAMA_SET_ROWS=1
        -e CUDA_VISIBLE_DEVICES=0,1
        -v /models:/models
        -p ${PORT}:${PORT}
        ghcr.io/ggml-org/llama.cpp:full-cuda
        --host 0.0.0.0 --port ${PORT}
        -m /models/Qwen3-Coder-Next-MXFP4_MOE/Qwen3-Coder-Next-MXFP4_MOE.gguf
        -c 131072
        --fit on
        --flash-attn on
        --cache-type-k q4_0 --cache-type-v q4_0
        --no-webui
        --jinja --reasoning-format none
        --seed 3407 --temp 1.0 --top-p 0.95 --min-p 0.01 --top-k 40
    ttl: 900

  glm-4.7-dual:
    name: "GLM-4.7 Q4_K_XL"
    concurrencyLimit: 1
    fitPolicy: evict_to_fit
    initialVramMB: 46759
    initialCpuMB: 245760
    cmd: >
      docker run --rm --gpus all
        -e LLAMA_SET_ROWS=1
        -e CUDA_VISIBLE_DEVICES=0,1
        -v /models:/models
        -p ${PORT}:${PORT}
        ghcr.io/ggml-org/llama.cpp:full-cuda
        --host 0.0.0.0 --port ${PORT}
        -m /models/GLM-4.7-Q4_K_XL/UD-Q4_K_XL/GLM-4.7-UD-Q4_K_XL-00001-of-00005.gguf
        -c 131072
        --flash-attn on
        --fit on
        --cpu-moe
        --cache-type-k q4_0 --cache-type-v q4_0
        --batch-size 8192 --ubatch-size 2048
        --no-webui
        --jinja --temp 1.0 --top-p 0.95
    ttl: 900

  deepseek-v3-dual:
    name: "DeepSeek v3.2"
    concurrencyLimit: 1
    fitPolicy: evict_to_fit
    initialVramMB: 46759
    initialCpuMB: 245760
    cmd: >
      docker run --rm --gpus all
        -e LLAMA_SET_ROWS=1
        -e CUDA_VISIBLE_DEVICES=0,1
        -v /models:/models
        -p ${PORT}:${PORT}
        ghcr.io/ggml-org/llama.cpp:full-cuda
        --host 0.0.0.0 --port ${PORT}
        -m /models/DeepSeek-V3.2-GGUF-IQ3_XXS/UD-IQ3_XXS/DeepSeek-V3.2-UD-IQ3_XXS-00001-of-00006.gguf
        --fit on
        --cpu-moe
        -c 131072
        --flash-attn on
        --cache-type-k q4_0 --cache-type-v q4_0
        --batch-size 2048 --ubatch-size 512
        --no-webui
        --jinja --temp 0.6 --top-p 0.95 --min-p 0.01 --seed 3407
    ttl: 900
```



## Logging notes

- At `logLevel: info`, proxy lifecycle messages are shown but HTTP access request logs are hidden.
- HTTP access request logs are emitted at `logLevel: debug`.
- To include upstream model stdout in terminal output, set `logToStdout: both` (or `upstream`).

## Why these scheduler settings matter

- `gpuVramCapsMB`: tells the scheduler each GPU has 24GB of VRAM, even if the driver reports more or you want to reserve headroom.
- `hostRamCapMB`: prevents multiple heavy models from being scheduled together beyond the system RAM cap.
- `initialVramMB`: seeds the scheduler with a VRAM estimate (95% of 24GB for single GPU models, 95% of 48GB for dual GPU models) before runtime measurements are available.
- `initialCpuMB`: gives the scheduler an immediate RAM estimate for huge models (like GLM-4.7 and DeepSeek v3.2) before runtime measurements are available.

If you need more headroom for other models, reduce `initialCpuMB` or increase `hostRamCapMB` to match your system.
