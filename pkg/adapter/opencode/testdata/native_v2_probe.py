"""Opt-in actual OpenCode V2 host test with a loopback-only synthetic provider."""
import json
import base64
import re
import os
import signal
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from native_v2_probe_stub import Stub


AUTH_HEADER = None

def request(base, method, path, body=None, timeout=15):
    data = None if body is None else json.dumps(body).encode()
    headers={"Content-Type":"application/json"}
    if AUTH_HEADER:
        headers["Authorization"]=AUTH_HEADER
    req = urllib.request.Request(base + path, data=data, method=method, headers=headers)
    opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open(req, timeout=timeout) as response:
        raw = response.read(2 * 1024 * 1024 + 1)
        if len(raw) > 2 * 1024 * 1024:
            raise RuntimeError("native API response exceeds fixture bound")
        return json.loads(raw) if raw else None


def isolated_environment(root):
    env = {k: os.environ[k] for k in ("PATH", "HOME", "LANG", "LC_ALL", "TMPDIR", "USER") if k in os.environ}
    for key, suffix in (("XDG_CONFIG_HOME", "config"), ("XDG_DATA_HOME", "data"), ("XDG_CACHE_HOME", "cache"), ("XDG_STATE_HOME", "state")):
        path = root / "isolated" / suffix
        path.mkdir(parents=True, exist_ok=True)
        env[key] = str(path)
    env["npm_config_cache"] = str(root / "isolated" / "npm-cache")
    global_fixture = Path(env["XDG_CONFIG_HOME"]) / "opencode"
    global_fixture.mkdir()
    (global_fixture / "opencode.json").write_text("{}")
    return env


def main():
    global AUTH_HEADER
    root, binary, logs = Path(sys.argv[1]), sys.argv[2], Path(sys.argv[3])
    logs.mkdir(parents=True, exist_ok=True)
    env = isolated_environment(root)
    version = subprocess.run([binary, "--version"], capture_output=True, text=True, timeout=10).stdout.strip()
    if "2.0." not in version:
        raise RuntimeError("this fixture requires installed OpenCode 2.0.x")
    work = root / "work"
    work.mkdir()
    stub = Stub(work)
    server = None
    sessions = []
    base = None
    report = {"scope": "native_hook_dispatch_with_synthetic_loopback_model", "runtime": version, "scenarios": []}
    server_log = open(logs / "server.log", "w")
    try:
        # No package.json, node_modules, npm install, or unit-test shim: the
        # generated plugin must load through the installed native host itself.
        report["sdk_source"]="native_plugin_object_no_sdk_or_shim"
        config = {"model": "fixture/hook-fixture", "providers": {"fixture": {"package": "@opencode/ai/providers/openai-compatible", "settings": {"baseURL": stub.url, "apiKey": "synthetic-fixture-only"}, "models": {"hook-fixture": {"modelID": "hook-fixture"}}}}, "permissions": [{"action": "shell", "resource": "*", "effect": "allow"}, {"action": "read", "resource": "*", "effect": "allow"}]}
        (root / "opencode.json").write_text(json.dumps(config))
        with socket.socket() as lease:
            lease.bind(("127.0.0.1", 0))
            port = lease.getsockname()[1]
        base = "http://127.0.0.1:" + str(port)
        server = subprocess.Popen([binary, "serve", "--hostname", "127.0.0.1", "--port", str(port)], cwd=root, env=env, stdout=server_log, stderr=subprocess.STDOUT, start_new_session=True)
        deadline = time.monotonic() + 25
        while time.monotonic() < deadline:
            if server.poll() is not None:
                raise RuntimeError("owned OpenCode server exited during startup")
            try:
                bootstrap=(logs / "server.log").read_text()
                match=re.search(r"^server password (\S+)$",bootstrap,re.MULTILINE)
                if not match or base not in bootstrap:
                    time.sleep(.1)
                    continue
                AUTH_HEADER="Basic "+base64.b64encode(("opencode:"+match.group(1)).encode()).decode()
                if request(base, "GET", "/api/info", timeout=1):
                    break
            except (OSError, urllib.error.HTTPError):
                time.sleep(.1)
        else:
            raise RuntimeError("owned OpenCode server readiness timeout")
        location = "?" + urllib.parse.urlencode({"location": json.dumps({"directory": str(root)})})
        for blocked in (False, True):
            for path in (work / "lifecycle.log", work / "shell-ran", work / "reject-before"):
                path.unlink(missing_ok=True)
            if blocked:
                (work / "reject-before").touch()
            created = request(base, "POST", "/api/session" + location, {"title": "autopus-native-hook-fixture", "location":{"directory":str(root)}, "model": {"providerID": "fixture", "id": "hook-fixture"}})
            session = created["data"]["id"]
            if not session.startswith("ses"):
                raise RuntimeError("native session ID unverified")
            sessions.append(session)
            request(base, "POST", "/api/session/" + session + "/prompt", {"text": "NATIVE_FIXTURE_BLOCK" if blocked else "NATIVE_FIXTURE_NORMAL"})
            request(base, "POST", "/api/experimental/session/" + session + "/wait", {}, timeout=25)
            actual = (work / "lifecycle.log").read_text() if (work / "lifecycle.log").exists() else ""
            expected = "before\n" if blocked else "before\ntool\nafter\n"
            if actual != expected or (work / "shell-ran").exists() == blocked:
                raise RuntimeError("native hook scenario mismatch: " + json.dumps({"blocked": blocked, "actual": actual, "shell_ran": (work / "shell-ran").exists()}))
            if (root / "lifecycle.log").exists():
                raise RuntimeError("hook used plugin cwd instead of native session shell workdir")
            report["scenarios"].append({"blocked": blocked, "order": actual.splitlines(), "shell_ran": not blocked})
        if stub.errors:
            raise RuntimeError("; ".join(stub.errors))
        report["passed"] = True
        print("NATIVE_V2_HOOKS_PASS")
    finally:
        report["stub_requests"] = stub.requests
        report["stub_errors"] = stub.errors
        if server is not None:
            for session in sessions:
                try:
                    request(base, "DELETE", "/api/session/" + session, timeout=2)
                except Exception:
                    pass
            if server.poll() is None:
                os.killpg(server.pid, signal.SIGTERM)
                try:
                    server.wait(timeout=3)
                except subprocess.TimeoutExpired:
                    os.killpg(server.pid, signal.SIGKILL)
                    server.wait(timeout=3)
        stub.close()
        server_log.close()
        server_path=logs / "server.log"
        server_path.write_text(re.sub(r"(?m)^server password .*", "server password [redacted task-only bootstrap]", server_path.read_text()))
        report["process_cleanup"]={"server_exited":server is None or server.poll() is not None,"stub_stopped":not stub.thread.is_alive()}
        (logs / "report.json").write_text(json.dumps(report, indent=2))


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(143))
    main()
