#!/usr/bin/env python3
"""memos2txt pyannote diarization helper.

Reads a 16 kHz mono WAV file, runs a pyannote speaker diarization pipeline,
and writes deterministic JSON turns. Progress goes to stderr; stdout stays quiet.

This file is embedded into the memos2txt binary and written into the managed
pyannote environment at runtime.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import traceback


def log(message: str) -> None:
    print(f"[diarize] {message}", file=sys.stderr, flush=True)


def fail(message: str, code: int = 2) -> "None":
    log(f"error: {message}")
    raise SystemExit(code)


def choose_device(torch, requested: str) -> tuple[str, str | None]:
    requested = (requested or "auto").strip().lower()
    available = ["cpu"]
    if torch.backends.mps.is_available():
        available.append("mps")
    if torch.cuda.is_available():
        available.append("cuda")

    if requested == "auto":
        if "cuda" in available:
            return "cuda", None
        if "mps" in available:
            return "mps", None
        return "cpu", None

    if requested not in available:
        return "cpu", f"requested device {requested!r} is unavailable; using cpu"
    return requested, None


def canonical_speakers(turns: list[dict]) -> list[dict]:
    mapping: dict[str, str] = {}
    for turn in turns:
        raw = str(turn["speaker"])
        if raw not in mapping:
            mapping[raw] = f"SPEAKER_{len(mapping):02d}"
        turn["speaker"] = mapping[raw]
    return turns


def summarize(turns: list[dict]) -> list[dict]:
    stats: dict[str, dict] = {}
    for turn in turns:
        speaker = turn["speaker"]
        item = stats.setdefault(speaker, {"id": speaker, "turns": 0, "speech_seconds": 0.0})
        item["turns"] += 1
        item["speech_seconds"] += max(0.0, float(turn["end"]) - float(turn["start"]))
    out = []
    for speaker in sorted(stats):
        item = stats[speaker]
        item["speech_seconds"] = round(item["speech_seconds"], 2)
        out.append(item)
    return out


def extract_annotation(output):
    annotation = getattr(output, "exclusive_speaker_diarization", None)
    if annotation is not None:
        return annotation, "exclusive"
    annotation = getattr(output, "speaker_diarization", None)
    if annotation is not None:
        return annotation, "regular"
    return output, "annotation"


def main() -> None:
    parser = argparse.ArgumentParser(description="pyannote diarization helper for memos2txt")
    parser.add_argument("--audio", required=True)
    parser.add_argument("--model", default="pyannote/speaker-diarization-community-1")
    parser.add_argument("--output", required=True)
    parser.add_argument("--device", default="auto")
    parser.add_argument("--num-speakers", type=int, default=None)
    parser.add_argument("--min-speakers", type=int, default=None)
    parser.add_argument("--max-speakers", type=int, default=None)
    args = parser.parse_args()

    started = time.time()

    token = (
        os.environ.get("HF_TOKEN")
        or os.environ.get("HUGGING_FACE_HUB_TOKEN")
        or os.environ.get("HUGGINGFACE_TOKEN")
        or ""
    ).strip()

    model_path = os.path.expanduser(args.model)
    is_local_model = os.path.isdir(model_path) or os.path.isfile(model_path)

    if not token and not is_local_model:
        fail(
            "missing Hugging Face token. Accept the model terms and run "
            "`memos2txt auth huggingface --api-key-stdin` or set HF_TOKEN."
        )

    if not os.path.isfile(args.audio):
        fail(f"audio file not found: {args.audio}")

    try:
        import numpy as np
        import soundfile as sf
        import torch
        from pyannote.audio import Pipeline
    except Exception as exc:  # pragma: no cover - dependency diagnostics
        fail(f"failed to import diarization dependencies: {exc}")

    device, note = choose_device(torch, args.device)
    if note:
        log(note)

    log(f"loading model {args.model}")
    load_started = time.time()
    try:
        pipeline = Pipeline.from_pretrained(args.model, token=token or None)
    except Exception as exc:
        fail(f"failed to load pipeline {args.model}: {exc}")
    if pipeline is None:
        fail(
            f"pipeline {args.model} could not be loaded. Confirm that the model terms "
            "have been accepted on Hugging Face and that HF_TOKEN is valid."
        )
    log(f"model loaded in {time.time() - load_started:.2f}s")

    try:
        pipeline.to(torch.device(device))
    except Exception as exc:
        if device == "cpu":
            fail(f"failed to place pipeline on cpu: {exc}")
        log(f"failed to use {device} ({exc}); falling back to cpu")
        device = "cpu"
        pipeline.to(torch.device("cpu"))

    log("loading audio")
    try:
        data, sample_rate = sf.read(args.audio, dtype="float32", always_2d=True)
    except Exception as exc:
        fail(f"failed to read audio: {exc}")

    waveform = torch.from_numpy(np.ascontiguousarray(data.T))
    if waveform.shape[0] > 1:
        waveform = waveform.mean(dim=0, keepdim=True)
    duration = float(waveform.shape[1]) / float(sample_rate)
    log(f"audio duration {duration:.2f}s, sample_rate {sample_rate}, device {device}")

    kwargs: dict = {}
    if args.num_speakers is not None and args.num_speakers > 0:
        kwargs["num_speakers"] = args.num_speakers
    else:
        if args.min_speakers is not None and args.min_speakers > 0:
            kwargs["min_speakers"] = args.min_speakers
        if args.max_speakers is not None and args.max_speakers > 0:
            kwargs["max_speakers"] = args.max_speakers

    log("running diarization")
    infer_started = time.time()
    try:
        with torch.inference_mode():
            output = pipeline(
                {"waveform": waveform, "sample_rate": sample_rate},
                **kwargs,
            )
    except RuntimeError as exc:
        if device == "mps":
            log(f"mps inference failed ({exc}); retrying on cpu")
            device = "cpu"
            pipeline.to(torch.device("cpu"))
            with torch.inference_mode():
                output = pipeline(
                    {"waveform": waveform, "sample_rate": sample_rate},
                    **kwargs,
                )
        else:
            raise
    infer_seconds = time.time() - infer_started

    annotation, source = extract_annotation(output)
    turns: list[dict] = []
    try:
        iterator = annotation.itertracks(yield_label=True)
        for turn, _track, speaker in iterator:
            start = float(turn.start)
            end = float(turn.end)
            if end <= start:
                continue
            turns.append({"start": round(start, 3), "end": round(end, 3), "speaker": str(speaker)})
    except AttributeError:
        for turn, speaker in annotation:
            start = float(turn.start)
            end = float(turn.end)
            if end <= start:
                continue
            turns.append({"start": round(start, 3), "end": round(end, 3), "speaker": str(speaker)})

    turns.sort(key=lambda item: (item["start"], item["end"]))
    turns = canonical_speakers(turns)
    speakers = summarize(turns)

    result = {
        "ok": True,
        "model": args.model,
        "device": device,
        "source": source,
        "audio_seconds": round(duration, 3),
        "speaker_count": len(speakers),
        "inference_seconds": round(infer_seconds, 3),
        "seconds": round(time.time() - started, 3),
        "turns": turns,
        "speakers": speakers,
    }

    output_path = os.path.abspath(args.output)
    os.makedirs(os.path.dirname(output_path), exist_ok=True)
    tmp_path = output_path + ".tmp"
    with open(tmp_path, "w", encoding="utf-8") as handle:
        json.dump(result, handle, ensure_ascii=False, indent=2)
    os.replace(tmp_path, output_path)
    try:
        os.chmod(output_path, 0o600)
    except OSError:
        pass

    log(f"diarization complete: {len(speakers)} speakers, {len(turns)} turns in {infer_seconds:.2f}s")


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except BaseException:
        traceback.print_exc(file=sys.stderr)
        raise SystemExit(2)
