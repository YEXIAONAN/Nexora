#!/usr/bin/env python3
"""Real HTTP fixture/verification helper for the M6 split backup restore drill.

No database rows are fabricated: the seed path uses the same public API and
permissions as the browser workflow. Only an isolated test database and a
temporary attachment directory are touched.
"""
import argparse
import hashlib
import json
import sys
import traceback
import uuid
from http.cookiejar import CookieJar
from urllib.error import HTTPError, URLError
from urllib.request import HTTPCookieProcessor, ProxyHandler, Request, build_opener

MUTATING = {"POST", "PUT", "PATCH", "DELETE"}


class Api:
    def __init__(self, base):
        self.base = base.rstrip("/")
        self.jar = CookieJar()
        # Never route loopback test traffic through an operator's system proxy.
        self.opener = build_opener(ProxyHandler({}), HTTPCookieProcessor(self.jar))

    def request(self, method, path, body=None, raw=None, headers=None):
        hdrs = {"Accept": "application/json"}
        if method in MUTATING:
            hdrs["X-Nexora-CSRF"] = "1"
        if headers:
            hdrs.update(headers)
        if raw is not None:
            data = raw
        elif body is not None:
            data = json.dumps(body).encode()
            hdrs["Content-Type"] = "application/json"
        else:
            data = None
        req = Request(self.base + path, data=data, method=method, headers=hdrs)
        try:
            with self.opener.open(req, timeout=30) as response:
                payload = response.read()
                return response.status, payload, response.headers
        except HTTPError as error:
            return error.code, error.read(), error.headers

    def json(self, method, path, body=None, headers=None, raw=None):
        status, payload, _ = self.request(method, path, body=body, raw=raw, headers=headers)
        if status < 200 or status >= 300:
            raise RuntimeError(f"{method} {path} -> {status}: {payload[:500]!r}")
        return json.loads(payload)["data"]


def login(api, username, password):
    return api.json("POST", "/api/v1/auth/login", {"username": username, "password": password})[
        "user"
    ]


def project_path(team, project):
    return f"/api/v1/teams/{team}/projects/{project}"


def case_path(team, project, case):
    return f"{project_path(team, project)}/contributions/{case}"


def task_path(team, project, task):
    return f"{project_path(team, project)}/tasks/{task}"


def contribution_command(case, reason):
    return {
        "expected_version": case["case"]["revision"],
        "client_request_id": str(uuid.uuid4()),
        "reason": reason,
        "case_version_id": case["latest"]["id"],
    }


def seed(args):
    root = Api(args.base_url)
    root_identity = login(root, "root", args.password)
    team = root.json(
        "POST",
        "/api/v1/teams",
        {"name": "M6 split restore team", "description": "real API fixture"},
    )
    people = {}
    for username, display in [
        ("m6_worker", "M6 成果作者"),
        ("m6_reviewer", "M6 独立验收人"),
        ("m6_appraiser", "M6 独立核定人"),
    ]:
        user = root.json(
            "POST",
            "/api/v1/admin/users",
            {"username": username, "display_name": display, "password": args.password},
        )
        invitation = root.json(
            "POST",
            f"/api/v1/teams/{team['id']}/invitations",
            {"username": username, "role": "member"},
        )
        client = Api(args.base_url)
        login(client, username, args.password)
        client.json("POST", f"/api/v1/invitations/{invitation['id']}/accept")
        people[username] = {"id": user["id"], "api": client}

    # m6_outsider is deliberately NOT a member of the owning Team. The account
    # only joins a second Team, so the restored 404s prove real cross-Team
    # isolation instead of the weaker "Team member but not project member" case.
    outsider_user = root.json(
        "POST",
        "/api/v1/admin/users",
        {
            "username": "m6_outsider",
            "display_name": "M6 其他团队成员",
            "password": args.password,
        },
    )
    outsider_api = Api(args.base_url)
    login(outsider_api, "m6_outsider", args.password)
    other_team = root.json(
        "POST",
        "/api/v1/teams",
        {"name": "M6 other team", "description": "cross-team denial"},
    )
    outsider_invitation = root.json(
        "POST",
        f"/api/v1/teams/{other_team['id']}/invitations",
        {"username": "m6_outsider", "role": "member"},
    )
    outsider_api.json("POST", f"/api/v1/invitations/{outsider_invitation['id']}/accept")
    people["m6_outsider"] = {"id": outsider_user["id"], "api": outsider_api}

    owning_members = {
        member["user_id"]
        for member in root.json("GET", f"/api/v1/teams/{team['id']}/members")
    }
    other_members = {
        member["user_id"]
        for member in root.json("GET", f"/api/v1/teams/{other_team['id']}/members")
    }
    if outsider_user["id"] in owning_members:
        raise RuntimeError("m6_outsider unexpectedly belongs to the owning Team")
    if outsider_user["id"] not in other_members:
        raise RuntimeError("m6_outsider did not join the second Team")
    for username in ("m6_worker", "m6_reviewer", "m6_appraiser"):
        if people[username]["id"] not in owning_members:
            raise RuntimeError(f"{username} is missing from the owning Team")

    project = root.json(
        "POST",
        f"/api/v1/teams/{team['id']}/projects",
        {"name": "M6 联合恢复项目", "lead_id": root_identity["id"]},
    )
    for username in ("m6_worker", "m6_reviewer", "m6_appraiser"):
        root.json(
            "POST",
            f"{project_path(team['id'], project['id'])}/members",
            {"user_id": people[username]["id"]},
        )
    root.json(
        "POST",
        f"{project_path(team['id'], project['id'])}/reviewers",
        {"user_id": people["m6_reviewer"]["id"], "reason": "M6 独立验收"},
    )
    task = root.json(
        "POST",
        f"{project_path(team['id'], project['id'])}/tasks",
        {"title": "M6 真实附件验收任务", "owner_id": people["m6_worker"]["id"], "weight": 1},
    )
    worker = people["m6_worker"]["api"]
    worker_task = worker.json(
        "GET", task_path(team["id"], project["id"], task["id"])
    )
    worker_task = worker.json(
        "POST",
        f"{task_path(team['id'], project['id'], task['id'])}/transition",
        {
            "status": "in_progress",
            "reason": "M6 开始真实成果",
            "expected_version": worker_task["version"],
        },
    )
    content = b"M6 real private attachment evidence\n" * 3
    boundary = "----nexora-m6-" + uuid.uuid4().hex
    multipart = (
        f"--{boundary}\r\n"
        'Content-Disposition: form-data; name="file"; filename="m6-evidence.txt"\r\n'
        "Content-Type: text/plain\r\n\r\n"
    ).encode() + content + f"\r\n--{boundary}--\r\n".encode()
    attachment = worker.json(
        "POST",
        f"{task_path(team['id'], project['id'], task['id'])}/attachments",
        raw=multipart,
        headers={
            "Content-Type": f"multipart/form-data; boundary={boundary}",
            "X-Nexora-Upload-Id": str(uuid.uuid4()),
        },
    )
    submission = worker.json(
        "POST",
        f"{task_path(team['id'], project['id'], task['id'])}/submissions",
        {
            "client_request_id": str(uuid.uuid4()),
            "expected_version": worker_task["version"],
            "summary": "M6 真实附件成果说明",
            "attachment_ids": [attachment["id"]],
        },
    )
    reviewer_task = people["m6_reviewer"]["api"].json(
        "GET", task_path(team["id"], project["id"], task["id"])
    )
    people["m6_reviewer"]["api"].json(
        "POST",
        f"{task_path(team['id'], project['id'], task['id'])}/submissions/{submission['id']}/reviews",
        {
            "expected_version": reviewer_task["version"],
            "decision": "approve",
            "comment": "M6 独立核验真实附件与成果",
        },
    )
    accepted = root.json("GET", task_path(team["id"], project["id"], task["id"]))
    case = root.json(
        "POST",
        f"{project_path(team['id'], project['id'])}/contributions",
        {
            "task_id": task["id"],
            "review_id": accepted["accepted_review_id"],
            "client_request_id": str(uuid.uuid4()),
            "reason": "M6 基于真实验收建立贡献案例",
        },
    )
    detail = root.json("GET", case_path(team["id"], project["id"], case["id"]))
    root.json(
        "PATCH",
        f"{case_path(team['id'], project['id'], case['id'])}/draft",
        {
            **contribution_command(detail, "M6 人工整理贡献草案"),
            "summary": "M6 联合恢复用正式贡献草案",
            "evidence_snapshot": {"note": "真实附件与独立验收", "review_id": accepted["accepted_review_id"]},
        },
    )
    detail = root.json("GET", case_path(team["id"], project["id"], case["id"]))
    root.json(
        "PUT",
        f"{case_path(team['id'], project['id'], case['id'])}/shares/{people['m6_worker']['id']}",
        {
            **contribution_command(detail, "M6 人工 10000 基点"),
            "share_bps": 10000,
            "basis": "人工评价真实成果与附件",
            "statement": "成果作者人工说明",
            "evidence_refs": [
                {
                    "kind": "objective_fact",
                    "source": "accepted_submission",
                    "reference": accepted["accepted_review_id"],
                }
            ],
        },
    )
    detail = root.json("GET", case_path(team["id"], project["id"], case["id"]))
    root.json(
        "POST",
        f"{case_path(team['id'], project['id'], case['id'])}/submit",
        contribution_command(detail, "M6 提交独立核定"),
    )
    root.json(
        "POST",
        f"{project_path(team['id'], project['id'])}/contribution-appraisers",
        {
            "user_id": people["m6_appraiser"]["id"],
            "expected_version": 0,
            "client_request_id": str(uuid.uuid4()),
            "reason": "M6 明确独立核定授权",
        },
    )
    detail = people["m6_appraiser"]["api"].json(
        "GET", case_path(team["id"], project["id"], case["id"])
    )
    people["m6_appraiser"]["api"].json(
        "POST",
        f"{case_path(team['id'], project['id'], case['id'])}/confirm",
        contribution_command(detail, "M6 独立核定正式贡献"),
    )
    detail = root.json("GET", case_path(team["id"], project["id"], case["id"]))
    effective = detail["effective"]
    notice = root.json(
        "POST",
        f"{case_path(team['id'], project['id'], case['id'])}/notices",
        {
            "expected_version": detail["case"]["revision"],
            "client_request_id": str(uuid.uuid4()),
            "reason": "M6 准备本人结果告知",
            "case_version_id": effective["id"],
            "confirmation_id": effective["confirmation"]["id"],
            "recipient_id": people["m6_worker"]["id"],
        },
    )
    detail = worker.json("GET", case_path(team["id"], project["id"], case["id"]))
    results = worker.json(
        "GET",
        f"{project_path(team['id'], project['id'])}/contribution-results/mine",
    )
    if len(results) != 1 or results[0]["notice"]["id"] != notice["id"]:
        raise RuntimeError("worker did not receive exactly the prepared result")
    worker.json(
        "POST",
        f"{case_path(team['id'], project['id'], case['id'])}/notices/{notice['id']}/receipt",
        {
            "expected_version": detail["case"]["revision"],
            "client_request_id": str(uuid.uuid4()),
            "case_version_id": effective["id"],
            "confirmation_id": effective["confirmation"]["id"],
            "content_digest": effective["confirmation"]["version_digest"],
            "received": True,
        },
    )
    attachment_path = (
        f"{task_path(team['id'], project['id'], task['id'])}/submissions/"
        f"{submission['id']}/attachments/{attachment['id']}"
    )
    status, downloaded, _ = worker.request("GET", attachment_path)
    if status != 200 or downloaded != content:
        raise RuntimeError(f"seed attachment download failed: {status}")
    if people["m6_outsider"]["api"].request("GET", attachment_path)[0] != 404:
        raise RuntimeError("source cross-team attachment was not denied")
    if (
        people["m6_outsider"]["api"].request(
            "GET", case_path(team["id"], project["id"], case["id"])
        )[0]
        != 404
    ):
        raise RuntimeError("source cross-team case was not denied")
    if (
        people["m6_outsider"]["api"].request(
            "GET", f"{project_path(team['id'], project['id'])}/contribution-results/mine"
        )[0]
        != 404
    ):
        raise RuntimeError("source cross-team results were not denied")
    state = {
        "team_id": team["id"],
        "other_team_id": other_team["id"],
        "project_id": project["id"],
        "task_id": task["id"],
        "submission_id": submission["id"],
        "attachment_id": attachment["id"],
        "attachment_path": attachment_path,
        "attachment_name": attachment["file_name"],
        "attachment_size": len(content),
        "attachment_sha256": hashlib.sha256(content).hexdigest(),
        "case_id": case["id"],
        "confirmation_id": effective["confirmation"]["id"],
        "version_id": effective["id"],
        "version_digest": effective["confirmation"]["version_digest"],
        "notice_id": notice["id"],
        "worker": "m6_worker",
        "worker_id": people["m6_worker"]["id"],
        "outsider": "m6_outsider",
        "outsider_id": people["m6_outsider"]["id"],
    }
    with open(args.state_file, "w", encoding="utf-8") as handle:
        json.dump(state, handle, indent=2, sort_keys=True)
    print(json.dumps(state, indent=2, sort_keys=True))


def verify(args):
    with open(args.state_file, encoding="utf-8") as handle:
        state = json.load(handle)
    worker = Api(args.base_url)
    login(worker, state["worker"], args.password)
    results = worker.json(
        "GET",
        f"{project_path(state['team_id'], state['project_id'])}/contribution-results/mine",
    )
    if len(results) != 1:
        raise RuntimeError(f"restored results={len(results)}, want 1")
    result = results[0]
    if result["notice"]["id"] != state["notice_id"] or result["confirmation"]["id"] != state["confirmation_id"]:
        raise RuntimeError("restored result lost its notice/confirmation binding")
    if not result["notice"]["receipt"]:
        raise RuntimeError("restored result lost the explicit receipt")
    status, downloaded, _ = worker.request("GET", state["attachment_path"])
    if status != 200 or hashlib.sha256(downloaded).hexdigest() != state["attachment_sha256"]:
        raise RuntimeError(f"restored attachment download/hash failed: {status}")
    history = worker.json(
        "GET", f"{case_path(state['team_id'], state['project_id'], state['case_id'])}/history"
    )
    if not history["notices"] or not any(item.get("receipt") for item in history["notices"]):
        raise RuntimeError("restored history lost notice receipt evidence")
    outsider = Api(args.base_url)
    login(outsider, state["outsider"], args.password)
    # Membership is re-read from the restored database, not inferred from the
    # earlier 404s: the outsider must belong only to the second Team.
    root = Api(args.base_url)
    login(root, "root", args.password)
    owning_members = {
        member["user_id"]
        for member in root.json("GET", f"/api/v1/teams/{state['team_id']}/members")
    }
    other_members = {
        member["user_id"]
        for member in root.json("GET", f"/api/v1/teams/{state['other_team_id']}/members")
    }
    if state["outsider_id"] in owning_members:
        raise RuntimeError("restored outsider belongs to the owning Team")
    if state["outsider_id"] not in other_members:
        raise RuntimeError("restored outsider lost the second-Team membership")
    if state["worker_id"] not in owning_members:
        raise RuntimeError("restored worker lost the owning Team membership")
    if outsider.request("GET", f"/api/v1/teams/{state['other_team_id']}/members")[0] != 200:
        raise RuntimeError("restored outsider cannot read its own Team members")
    if outsider.request("GET", f"/api/v1/teams/{state['team_id']}/members")[0] != 404:
        raise RuntimeError("restored outsider can read the owning Team members")
    if outsider.request("GET", state["attachment_path"])[0] != 404:
        raise RuntimeError("restored cross-team attachment was not denied")
    if outsider.request("GET", case_path(state["team_id"], state["project_id"], state["case_id"]))[0] != 404:
        raise RuntimeError("restored cross-team case was not denied")
    if outsider.request(
        "GET", f"{project_path(state['team_id'], state['project_id'])}/contribution-results/mine"
    )[0] != 404:
        raise RuntimeError("restored cross-team results were not denied")
    anonymous = Api(args.base_url)
    if anonymous.request("GET", state["attachment_path"])[0] != 401:
        raise RuntimeError("anonymous restored attachment was not denied")
    print("restored business evidence and permissions verified")


def check_download(args):
    with open(args.state_file, encoding="utf-8") as handle:
        state = json.load(handle)
    worker = Api(args.base_url)
    login(worker, state["worker"], args.password)
    status, _, _ = worker.request("GET", state["attachment_path"])
    if status != args.expect_status:
        raise RuntimeError(f"download status={status}, want {args.expect_status}")
    print(f"download status {status} as expected")


def load(args):
    import concurrent.futures
    import statistics
    import time

    with open(args.state_file, encoding="utf-8") as handle:
        state = json.load(handle)
    path = f"{project_path(state['team_id'], state['project_id'])}/contribution-results/mine"
    import threading

    local = threading.local()
    statuses = []
    samples = []

    def one(_index):
        if not hasattr(local, "client"):
            local.client = Api(args.base_url)
            login(local.client, state["worker"], args.password)
        started = time.perf_counter()
        status, _, _ = local.client.request("GET", path)
        return status, time.perf_counter() - started

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        # Warm up each thread's authenticated client before collecting samples.
        list(pool.map(one, range(args.concurrency)))
        for status, elapsed in pool.map(one, range(args.requests)):
            statuses.append(status)
            samples.append(elapsed)
    if any(status != 200 for status in statuses):
        raise RuntimeError(f"load statuses include failures: {sorted(set(statuses))}")
    samples.sort()

    def percentile(fraction):
        return round(samples[min(len(samples) - 1, int(len(samples) * fraction))] * 1000, 2)

    print(json.dumps({
        "requests": len(samples),
        "concurrency": args.concurrency,
        "p50_ms": percentile(0.50),
        "p95_ms": percentile(0.95),
        "max_ms": round(samples[-1] * 1000, 2),
        "mean_ms": round(statistics.mean(samples) * 1000, 2),
        "statuses": sorted(set(statuses)),
    }, indent=2, sort_keys=True))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=["seed", "verify", "check-download", "load"])
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--password", required=True)
    parser.add_argument("--state-file", required=True)
    parser.add_argument("--expect-status", type=int, default=200)
    parser.add_argument("--requests", type=int, default=200)
    parser.add_argument("--concurrency", type=int, default=8)
    args = parser.parse_args()
    try:
        {"seed": seed, "verify": verify, "check-download": check_download, "load": load}[args.mode](args)
    except (RuntimeError, URLError, KeyError) as error:
        print(f"m6 flow failed: {error}", file=sys.stderr)
        traceback.print_exc()
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
