#!/usr/bin/env python3
import argparse
import fnmatch
import os
import re
import subprocess
import sys
import time
from pathlib import Path


COUNT_RE = re.compile(r"^(TOTAL|SKIP|PASS|XFAIL|FAIL|XPASS):\s+(\d+)\s*$", re.M)


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="Run tlsfuzzer app scripts locally with per-script timeout."
    )
    p.add_argument("--host", required=True, help="Target host/IP for tlsfuzzer -h")
    p.add_argument("--port", required=True, type=int, help="Target port for tlsfuzzer -p")
    p.add_argument("--out-dir", required=True, help="Output directory for logs and summaries")
    p.add_argument(
        "--tlsfuzzer-root",
        default="/private/tmp/tlsfuzzer-src",
        help="Path to tlsfuzzer git checkout",
    )
    p.add_argument(
        "--python-bin",
        default=sys.executable,
        help="Python interpreter to use for script execution",
    )
    p.add_argument(
        "--per-script-timeout-sec",
        type=int,
        default=120,
        help="Per script timeout in seconds (default: 120)",
    )
    p.add_argument(
        "--script-glob",
        default="test_tls13_*.py",
        help="Glob relative to tlsfuzzer/_apps, e.g. test_tls12_*.py",
    )
    p.add_argument(
        "--include",
        action="append",
        default=[],
        help="Optional fnmatch pattern(s) for script basenames",
    )
    p.add_argument(
        "--max-scripts",
        type=int,
        default=0,
        help="Optional cap for quick dry-runs (0 = no cap)",
    )
    return p.parse_args()


def script_to_log_name(script_path: Path) -> str:
    return script_path.stem.replace("_", "-") + ".log"


def discover_scripts(root: Path, script_glob: str, includes: list[str]) -> list[Path]:
    apps = root / "tlsfuzzer" / "_apps"
    scripts = sorted(apps.glob(script_glob))
    if includes:
        scripts = [
            script
            for script in scripts
            if any(fnmatch.fnmatch(script.name, pat) for pat in includes)
        ]
    return scripts


def parse_counts(text: str) -> dict[str, int]:
    out: dict[str, int] = {}
    for key, value in COUNT_RE.findall(text):
        out[key] = int(value)
    return out


def main() -> int:
    args = parse_args()

    root = Path(args.tlsfuzzer_root).resolve()
    out_dir = Path(args.out_dir).resolve()
    out_dir.mkdir(parents=True, exist_ok=True)

    scripts = discover_scripts(root, args.script_glob, args.include)
    if args.max_scripts and args.max_scripts > 0:
        scripts = scripts[: args.max_scripts]

    if not scripts:
        print(f"no scripts selected for glob={args.script_glob}", file=sys.stderr)
        return 2

    summary_tsv = out_dir / "summary.tsv"
    summary_txt = out_dir / "summary.txt"
    meta_txt = out_dir / "run_meta.txt"

    meta_txt.write_text(
        "\n".join(
            [
                f"host={args.host}",
                f"port={args.port}",
                f"tlsfuzzer_root={root}",
                f"python_bin={args.python_bin}",
                f"per_script_timeout_sec={args.per_script_timeout_sec}",
                f"selected_scripts={len(scripts)}",
                f"script_glob={args.script_glob}",
                ("include=" + ",".join(args.include)) if args.include else "include=",
            ]
        )
        + "\n",
        encoding="utf-8",
    )

    rows: list[dict[str, object]] = []
    started = time.time()
    env = os.environ.copy()
    env["PYTHONPATH"] = str(root) + (os.pathsep + env["PYTHONPATH"] if env.get("PYTHONPATH") else "")

    print(
        f"[tlsfuzzer] target={args.host}:{args.port} glob={args.script_glob} "
        f"scripts={len(scripts)} timeout={args.per_script_timeout_sec}s"
    )
    for idx, script in enumerate(scripts, start=1):
        log_path = out_dir / script_to_log_name(script)
        cmd = [args.python_bin, str(script), "-h", args.host, "-p", str(args.port)]
        t0 = time.time()
        status = "FAIL"
        rc = None
        timed_out = False
        output = ""
        try:
            cp = subprocess.run(
                cmd,
                cwd=str(root),
                env=env,
                capture_output=True,
                text=True,
                timeout=args.per_script_timeout_sec,
            )
            rc = cp.returncode
            output = (cp.stdout or "") + (cp.stderr or "")
            status = "PASS" if cp.returncode == 0 else "FAIL"
        except subprocess.TimeoutExpired as exc:
            timed_out = True
            status = "TIMEOUT"
            rc = 124
            stdout = exc.stdout.decode(errors="replace") if isinstance(exc.stdout, bytes) else (exc.stdout or "")
            stderr = exc.stderr.decode(errors="replace") if isinstance(exc.stderr, bytes) else (exc.stderr or "")
            output = (stdout or "") + (stderr or "")
            output += f"\n[tlsfuzzer-runner] TIMEOUT after {args.per_script_timeout_sec}s\n"

        dur = time.time() - t0
        log_path.write_text(output, encoding="utf-8", errors="replace")
        counts = parse_counts(output)
        row = {
            "script": script.name,
            "status": status,
            "rc": rc,
            "duration_sec": f"{dur:.3f}",
            "TOTAL": counts.get("TOTAL", 0),
            "PASS": counts.get("PASS", 0),
            "FAIL": counts.get("FAIL", 0),
            "SKIP": counts.get("SKIP", 0),
            "XFAIL": counts.get("XFAIL", 0),
            "XPASS": counts.get("XPASS", 0),
            "timed_out": 1 if timed_out else 0,
        }
        rows.append(row)
        print(
            f"[{idx:02d}/{len(scripts):02d}] {script.name} status={status} rc={rc} "
            f"dur={dur:.1f}s probes(PASS/FAIL)={row['PASS']}/{row['FAIL']}"
        )

    cols = [
        "script",
        "status",
        "rc",
        "duration_sec",
        "TOTAL",
        "PASS",
        "FAIL",
        "SKIP",
        "XFAIL",
        "XPASS",
        "timed_out",
    ]
    with summary_tsv.open("w", encoding="utf-8") as handle:
        handle.write("\t".join(cols) + "\n")
        for row in rows:
            handle.write("\t".join(str(row[col]) for col in cols) + "\n")

    script_pass = sum(1 for row in rows if row["status"] == "PASS")
    script_fail = sum(1 for row in rows if row["status"] == "FAIL")
    script_timeout = sum(1 for row in rows if row["status"] == "TIMEOUT")
    probe_total = sum(int(row["TOTAL"]) for row in rows)
    probe_pass = sum(int(row["PASS"]) for row in rows)
    probe_fail = sum(int(row["FAIL"]) for row in rows)
    probe_skip = sum(int(row["SKIP"]) for row in rows)
    elapsed = time.time() - started

    fail_list = [row["script"] for row in rows if row["status"] == "FAIL"]
    timeout_list = [row["script"] for row in rows if row["status"] == "TIMEOUT"]

    lines = [
        f"target={args.host}:{args.port}",
        f"suite=tlsfuzzer glob={args.script_glob}",
        f"selected_scripts={len(scripts)}",
        f"per_script_timeout_sec={args.per_script_timeout_sec}",
        f"elapsed_sec={elapsed:.1f}",
        "",
        "script_summary:",
        f"- pass={script_pass}",
        f"- fail={script_fail}",
        f"- timeout={script_timeout}",
        "",
        "probe_summary:",
        f"- total={probe_total}",
        f"- pass={probe_pass}",
        f"- fail={probe_fail}",
        f"- skip={probe_skip}",
        "",
        "artifacts:",
        f"- summary_tsv={summary_tsv}",
        f"- run_meta={meta_txt}",
        f"- logs_dir={out_dir}",
    ]
    if fail_list:
        lines += ["", "failed_scripts:"] + [f"- {script}" for script in fail_list]
    if timeout_list:
        lines += ["", "timed_out_scripts:"] + [f"- {script}" for script in timeout_list]

    summary_txt.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"[tlsfuzzer] summary={summary_txt}")
    return 0 if (script_fail == 0 and script_timeout == 0) else 1


if __name__ == "__main__":
    raise SystemExit(main())
