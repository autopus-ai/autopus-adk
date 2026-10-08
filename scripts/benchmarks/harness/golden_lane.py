"""Signed-lane inputs of the golden runner (SPEC-HARNEVAL-003 T13).

`run.py --mode golden --signed-lane <lane.json>` runs the session the signer re-derives. The lane
file carries the five protocol fields the signed lane requires and a maintainer host never writes:
run_id and run_attempt (the Actions run), binding_digest and runner_tree_digest (computed by the
bind job's `auto`), and baseline_commit, which must still be the commit live.baseline_ref names;
the baseline arm is then built from that commit. Only active agent tasks with a black-box oracle
enter the session, graded by golden_blackbox_trial through the trusted oracle harness built from
this checkout; white-box tasks stay in the SPEC-HARNEVAL-001 advisory lane.
"""
import atexit
from dataclasses import replace
from functools import partial
import json
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
from types import SimpleNamespace

import golden_blackbox as gb
import golden_blackbox_trial as gbt
import golden_sandbox as gs
from observe import reject_json_constant, unique_object

FIELDS = ('run_id', 'run_attempt', 'binding_digest', 'baseline_commit', 'runner_tree_digest')
DIGEST, COMMIT = re.compile(r'^[0-9a-f]{64}$'), re.compile(r'^[0-9a-f]{40}$')


class LaneError(Exception):
    """A signed-lane input the runner refuses before any agent call (reason `invalid`)."""

    def __init__(self, detail: str):
        super().__init__(detail)
        self.detail = detail


def load(path: Path) -> dict:
    """The lane file: exactly the five fields, positive run numbers and lowercase hex digests."""
    try:
        document = json.loads(Path(path).read_text(), object_pairs_hook=unique_object,
                              parse_constant=reject_json_constant)
    except (OSError, ValueError) as error:
        raise LaneError('signed lane file: ' + str(error)) from error
    if not isinstance(document, dict) or set(document) != set(FIELDS):
        raise LaneError('signed lane file needs exactly ' + ', '.join(FIELDS))
    valid = all(type(document[key]) is int and document[key] > 0 for key in FIELDS[:2]) and \
        all(isinstance(document[key], str) and DIGEST.match(document[key]) for key in (FIELDS[2], FIELDS[4])) and \
        isinstance(document['baseline_commit'], str) and COMMIT.match(document['baseline_commit'])
    if not valid:
        raise LaneError('signed lane file: run numbers must be positive integers, digests lowercase hex')
    return {key: document[key] for key in FIELDS}


def resolve_commit(repo: Path, ref: str) -> str:
    """The 40-hex commit `ref` names in repo; empty when git cannot resolve it."""
    completed = subprocess.run(['git', 'rev-parse', '--verify', '--end-of-options', ref + '^{commit}'], cwd=repo,
                               capture_output=True, text=True, timeout=60)
    return completed.stdout.strip() if completed.returncode == 0 else ''


def black_box_tasks(set_root: Path, manifest: dict, tasks: list) -> list:
    """The active agent tasks whose committed task document is black-box, each with its validated oracle."""
    documents = {}
    for active in manifest['active_paths']:
        for path in sorted((Path(set_root) / active).rglob('*.json')):
            document = json.loads(path.read_text())
            if isinstance(document, dict) and document.get('kind') == 'agent':
                documents[document['id']] = document
    selected = []
    for task in tasks:
        try:
            oracle = gb.definition(documents[task['id']])
        except (KeyError, ValueError) as error:
            raise LaneError(task['id'] + ': ' + str(error)) from error
        if oracle is not None:
            selected.append({**task, 'black_box': oracle})
    return selected


def exclude_harness(source: Path) -> None:
    """Remove evals/harness/** from the snapshot every agent, grade and build copy is cloned from, so no
    agent, build or artifact sees an oracle input or expected output (REQ-HR-08)."""
    target = Path(source) / 'evals' / 'harness'
    if target.is_symlink() or target.is_file():
        target.unlink()
    elif target.is_dir():
        shutil.rmtree(target)


def signed(options, manifest: dict, tasks: list, steps):
    """(tasks, steps) of a signed-lane session, or both unchanged without --signed-lane."""
    if not options.signed_lane:
        return tasks, steps
    fields = load(options.signed_lane)
    commit = resolve_commit(options.repo, manifest['live']['baseline_ref'])
    if commit != fields['baseline_commit']:
        raise LaneError(f"baseline_commit {fields['baseline_commit']} is not {manifest['live']['baseline_ref']} "
                        f"({commit or 'unresolved'})")
    selected = black_box_tasks(options.dir, manifest, tasks)
    if not selected:
        raise LaneError('no active agent task has a black-box oracle')
    holder = tempfile.mkdtemp(prefix='harneval-oracle-')
    atexit.register(shutil.rmtree, holder, True)
    try:
        oracle = gs.build_oracle(Path(holder))
    except (OSError, subprocess.SubprocessError) as error:
        raise LaneError('oracle harness build failed: ' + type(error).__name__) from error
    options.baseline_revision = commit
    context = SimpleNamespace(oracle=oracle, set_root=Path(options.dir).resolve(),
                              params=gs.guard(options.dir), out=options.output.resolve())
    return selected, replace(steps, calibrate=partial(gbt.calibrate, context), trial=partial(gbt.run_trial, context),
                             lane_fields=fields)
