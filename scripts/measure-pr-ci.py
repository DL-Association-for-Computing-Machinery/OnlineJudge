#!/usr/bin/env python3
"""Read-only estimate of CI churn and turnaround for one GitHub PR."""
import argparse
from collections import Counter
from datetime import datetime
import json
import subprocess
import sys


def github(path, *args, paginate=False):
    cmd = ["gh", "api", "--method", "GET"]
    if paginate:
        cmd += ["--paginate", "--slurp"]
    return json.loads(subprocess.check_output([*cmd, path, *args], text=True))


def belongs_to_pr(run, pr):
    links = run["pull_requests"]
    if links:
        return any(link["number"] == pr["number"] for link in links)
    # Actions can omit pull_requests. Report this fallback as an estimate.
    return (
        (run["head_repository"] or {}).get("id") == (pr["head"]["repo"] or {}).get("id")
        and run["head_repository"] is not None
        and run["head_branch"] == pr["head"]["ref"]
        and pr["created_at"] <= run["created_at"] <= (pr["closed_at"] or "9999")
    )


def summarize(runs, pr):
    matching = [run for run in runs if belongs_to_pr(run, pr)]
    latest = Counter(run["conclusion"] or run["status"] for run in matching)
    final = max(
        (run for run in matching if run["head_sha"] == pr["head"]["sha"]),
        key=lambda run: (run["created_at"], run["id"]),
        default=None,
    )
    return {
        "workflow_runs": len(matching),
        "manual_retries": sum(run["run_attempt"] - 1 for run in matching),
        "latest_outcomes": dict(latest),
        "inferred_associations": sum(not run["pull_requests"] for run in matching),
        "latest_final_head_run": final,
    }


def elapsed_seconds(start, end):
    return int((datetime.fromisoformat(end) - datetime.fromisoformat(start)).total_seconds())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("pr", type=int, help="PR number in the current GitHub repository")
    args = parser.parse_args()
    if args.pr < 1:
        parser.error("PR number must be positive")
    root = "repos/{owner}/{repo}"
    pr = github(f"{root}/pulls/{args.pr}")
    pages = github(f"{root}/actions/workflows/governance.yml/runs",
                   "-f", "event=pull_request", "-f", f"branch={pr['head']['ref']}",
                   "-F", "per_page=100", paginate=True)
    runs = [run for page in pages for run in page["workflow_runs"]]
    summary = summarize(runs, pr)
    summary["history_may_be_truncated"] = bool(pages and pages[0]["total_count"] >= 1000)
    final = summary.pop("latest_final_head_run")
    summary["head_sha"] = pr["head"]["sha"]
    summary["latest_final_head_run"] = None
    if final:
        attempt_url = f"{root}/actions/runs/{final['id']}/attempts/{final['run_attempt']}"
        result = {"url": final["html_url"], "attempt": final["run_attempt"],
                  "outcome": final["conclusion"] or final["status"]}
        if final["conclusion"] == "success":
            job_pages = github(f"{attempt_url}/jobs", "-F", "per_page=100", paginate=True)
            jobs = [job for page in job_pages for job in page["jobs"]]
            completed_at = max((job["completed_at"] for job in jobs if job["completed_at"]), default=None)
            if completed_at is None:
                raise ValueError("successful run has no completed jobs")
            attempt_started_at = final["run_started_at"]
            if final["run_attempt"] != 1:
                attempt_started_at = github(attempt_url)["run_started_at"]
            result["execution_seconds"] = elapsed_seconds(attempt_started_at, completed_at)
            if final["run_attempt"] == 1:
                result["turnaround_seconds"] = elapsed_seconds(final["created_at"], completed_at)
            result["executed_jobs"] = {
                job["name"]: elapsed_seconds(job["started_at"], job["completed_at"])
                for job in jobs if job["conclusion"] == "success"
            }
        summary["latest_final_head_run"] = result
    print(json.dumps(summary, indent=2, sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except (OSError, subprocess.SubprocessError, ValueError) as exc:
        sys.exit(f"Unable to read CI metrics: {exc}")
