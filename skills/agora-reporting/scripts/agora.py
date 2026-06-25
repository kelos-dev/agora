#!/usr/bin/env python3
"""Post and read Agora coordination events."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request


EVENT_TYPES = {
    "blocked",
    "ci_completed",
    "ci_failed",
    "ci_passed",
    "code_changed",
    "comment",
    "decision",
    "handoff",
    "instruction",
    "pr_opened",
    "question",
    "review_received",
    "summary",
    "tests_failed",
    "tests_passed",
}

STATUSES = {"acknowledged", "done", "open", "posted", "rejected", "resolved"}
ACTIONABLE_STATUSES = {"acknowledged", "open", "posted"}


def main() -> int:
    parser = argparse.ArgumentParser(description="Post and read Agora coordination events.")
    subcommands = parser.add_subparsers(dest="command", required=True)

    post = subcommands.add_parser("post", help="post an Agora event")
    post.add_argument("--type", required=True, choices=sorted(EVENT_TYPES))
    post.add_argument("--title", required=True)
    post.add_argument("--body", default="")
    post.add_argument("--thread")
    post.add_argument("--actor", default=default_actor())
    post.add_argument("--target", action="append", default=[])
    post.add_argument("--repo", default=default_repo())
    post.add_argument("--task", default=os.environ.get("AGORA_TASK", ""))
    post.add_argument("--severity", default="")
    post.add_argument("--reply-to", default="")
    post.add_argument("--link", action="append", default=[], metavar="NAME=URL")
    post.set_defaults(func=cmd_post)

    inbox = subcommands.add_parser("inbox", help="read this agent's Agora inbox")
    inbox.add_argument("--agent", default=default_actor())
    inbox.add_argument("--all", action="store_true", help="show closed and already handled events too")
    inbox.add_argument("--open", action="store_true", help="only show open and acknowledged events")
    inbox.add_argument("--limit", type=int, default=20)
    inbox.set_defaults(func=cmd_inbox)

    status = subcommands.add_parser("status", help="update an Agora event status")
    status.add_argument("event_id")
    status.add_argument("status", choices=sorted(STATUSES))
    status.add_argument("--actor", default=default_actor())
    status.set_defaults(func=cmd_status)

    args = parser.parse_args()
    if not os.environ.get("AGORA_URL"):
        print("AGORA_URL is not set; skipping Agora.", file=sys.stderr)
        return 0
    return args.func(args)


def cmd_post(args: argparse.Namespace) -> int:
    payload = {
        "type": args.type,
        "actor": args.actor,
        "thread": post_thread(args),
        "title": args.title,
        "body": args.body,
        "targets": split_targets(args.target),
        "repo": args.repo,
        "task": args.task,
        "severity": args.severity,
        "reply_to": args.reply_to,
        "links": parse_links(args.link),
    }
    event = request_json("POST", "/api/events", compact(payload))
    print(f"posted {event.get('id', '')} {event.get('type', '')} #{event.get('thread', '')}")
    return 0


def post_thread(args: argparse.Namespace) -> str:
    if args.thread is not None:
        return args.thread
    if args.reply_to:
        return ""
    return os.environ.get("AGORA_THREAD", "general")


def cmd_inbox(args: argparse.Namespace) -> int:
    query = urllib.parse.urlencode({"limit": str(args.limit)})
    events = request_json("GET", f"/api/agents/{urllib.parse.quote(args.agent)}/inbox?{query}")
    if args.open:
        events = [event for event in events if event.get("status") in {"acknowledged", "open"}]
    elif not args.all:
        events = [event for event in events if event.get("status") in ACTIONABLE_STATUSES]
    if not events:
        print("inbox empty")
        return 0
    for event in events:
        print(render_event(event))
    return 0


def cmd_status(args: argparse.Namespace) -> int:
    payload = {"actor": args.actor, "status": args.status}
    event = request_json("POST", f"/api/events/{urllib.parse.quote(args.event_id)}/status", payload)
    print(f"{event.get('id', args.event_id)} status={event.get('status', args.status)}")
    return 0


def request_json(method: str, path: str, payload: dict[str, object] | None = None) -> object:
    base = normalize_base_url(os.environ["AGORA_URL"])
    url = base + path
    data = None
    headers = {"Accept": "application/json"}
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"
    token = os.environ.get("AGORA_TOKEN")
    if token:
        headers["Authorization"] = f"Bearer {token}"
    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            return json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as err:
        detail = err.read().decode("utf-8", errors="replace")
        raise SystemExit(f"Agora request failed: HTTP {err.code}: {detail}") from err
    except urllib.error.URLError as err:
        raise SystemExit(f"Agora request failed: {err.reason}") from err


def normalize_base_url(value: str) -> str:
    value = value.strip().rstrip("/")
    parsed = urllib.parse.urlparse(value)
    if parsed.scheme:
        return value
    return f"http://{value}"


def default_actor() -> str:
    for key in ("AGORA_AGENT", "CODEX_AGENT", "USER"):
        value = os.environ.get(key)
        if value:
            return value
    return "agent"


def default_repo() -> str:
    remote = run_git("config", "--get", "remote.origin.url")
    if remote:
        return remote
    root = run_git("rev-parse", "--show-toplevel")
    if root:
        return os.path.basename(root)
    return os.path.basename(os.getcwd())


def run_git(*args: str) -> str:
    try:
        result = subprocess.run(
            ["git", *args],
            check=False,
            capture_output=True,
            text=True,
        )
    except FileNotFoundError:
        return ""
    if result.returncode != 0:
        return ""
    return result.stdout.strip()


def split_targets(values: list[str]) -> list[str]:
    targets: list[str] = []
    for value in values:
        for item in value.replace(",", " ").split():
            item = item.strip().removeprefix("@")
            if item and item not in targets:
                targets.append(item)
    return targets


def parse_links(values: list[str]) -> dict[str, str]:
    links: dict[str, str] = {}
    for value in values:
        name, sep, url = value.partition("=")
        if sep and name.strip() and url.strip():
            links[name.strip()] = url.strip()
    return links


def compact(payload: dict[str, object]) -> dict[str, object]:
    return {key: value for key, value in payload.items() if value not in ("", [], {})}


def render_event(event: dict[str, object]) -> str:
    title = event.get("title") or event.get("body") or event.get("type")
    rendered = (
        f"{event.get('id', '')} [{event.get('status', '')}] "
        f"{event.get('type', '')} from @{event.get('actor', '')} "
        f"#{event.get('thread', '')}: {title}"
    )
    body = event.get("body")
    if body:
        rendered += f"\n  {body}"
    return rendered


if __name__ == "__main__":
    raise SystemExit(main())
