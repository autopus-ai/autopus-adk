"""Pilot C preflight refusals and the frozen copy of the run's inputs.

A run refuses to start when its result could not mean what it claims:
  * the harness revision has a task's pre-review or fixed head as an ancestor, so the pinned `auto`
    on the agent's PATH would carry the reviewed code (the answer);
  * the output directory has an ancestor holding CLAUDE.md, CLAUDE.local.md or .claude/: Claude Code
    loads parent instructions and rules into every arm, native included;
  * the harness revision lacks a commit a task names in harness_requires (for c01/c02 the #348
    verify-stop budget fix, without which every stop after an internal/cli change blocks);
  * the output lies in a shared temporary root (/private/tmp, /private/var/tmp, $TMPDIR);
  * no passing gates.json (gate_c.py) for exactly this corpus and these oracles.
The profiles, oracles and corpus are copied into the output first, so switching the runner's branch
mid-run changes nothing.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess

from oracle_c import frozen_hash

HERE = Path(__file__).resolve().parent
PARENT_INSTRUCTIONS = ('CLAUDE.md', 'CLAUDE.local.md', '.claude')


def contains_task_head(repo: Path, harness: str, tasks: list) -> list:
    """Task heads (pre-review or fixed) that are ancestors of harness; an unknown head counts too."""
    found = []
    for task in tasks:
        for kind in ('pre_review', 'fixed'):
            head = task['heads'][kind]
            code = subprocess.run(['git', 'merge-base', '--is-ancestor', head, harness], cwd=repo,
                                  capture_output=True).returncode
            if code != 1:
                found.append('%s %s %s (%s)' % (task['id'], kind, head[:12], 'ancestor' if code == 0 else 'unknown'))
    return found


def missing_requirements(repo: Path, harness: str, tasks: list) -> list:
    """Commits a task needs in the harness (task['harness_requires']) that harness does not contain."""
    missing = []
    for task in tasks:
        for commit in task.get('harness_requires', []):
            code = subprocess.run(['git', 'merge-base', '--is-ancestor', commit, harness], cwd=repo,
                                  capture_output=True).returncode
            if code != 0:
                missing.append('%s needs %s' % (task['id'], commit[:12]))
    return missing


def shared_temp(output: Path) -> list:
    """Temporary roots other sessions write into (Claude Code keeps its cwd files and scratchpads in
    /private/tmp/claude-*); an output there could meet another session's files or symlinks."""
    output = Path(os.path.realpath(output))
    roots = [Path('/private/tmp'), Path('/private/var/tmp')]
    if os.environ.get('TMPDIR'):
        roots.append(Path(os.path.realpath(os.environ['TMPDIR'])))
    return [str(root) for root in roots if root == output or root in output.parents]


def parent_instructions(output: Path) -> list:
    """Ancestors of output (and output itself) that hold Claude Code project instructions."""
    output = Path(os.path.realpath(output))
    return [str(directory / name) for directory in [output, *output.parents] for name in PARENT_INSTRUCTIONS
            if (directory / name).exists()]


def check_gates(path: Path, tasks: list, oracles: Path) -> None:
    gates = json.loads(Path(path).read_text())
    if not gates.get('passed'):
        raise SystemExit('gates.json did not pass: ' + str(path))
    if gates.get('frozen_hash') != frozen_hash(tasks, oracles):
        raise SystemExit('gates.json was checked for another corpus or other oracles: ' + str(path))
    if sorted(gates.get('tasks', [])) != sorted(t['id'] for t in tasks):
        raise SystemExit('gates.json covers other tasks: ' + ', '.join(gates.get('tasks', [])))


def freeze(output: Path, corpus: Path) -> dict:
    """Copy the agent and grader profiles, the corpus and the oracles into output/frozen."""
    frozen = Path(output) / 'frozen'
    frozen.mkdir()
    for name in ('claude_agent.sb', 'grader.sb'):
        shutil.copyfile(HERE / name, frozen / name)
    shutil.copyfile(corpus, frozen / 'corpus_c.json')
    shutil.copytree(HERE / 'oracles', frozen / 'oracles')
    return frozen_paths(output)


def frozen_paths(output: Path) -> dict:
    frozen = Path(output) / 'frozen'
    return {'agent_profile': frozen / 'claude_agent.sb', 'grader_profile': frozen / 'grader.sb',
            'corpus': frozen / 'corpus_c.json', 'oracles': frozen / 'oracles'}


def preflight(repo: Path, output: Path, harness: str, tasks: list, gates: Path | None, oracles: Path,
              dry_run: bool) -> None:
    leaked = contains_task_head(repo, harness, tasks)
    if leaked:
        raise SystemExit('the harness revision contains task heads (choose one branched before them, e.g. the '
                         'last PR #348 commit 30722ad9 for c01/c02): ' + '; '.join(leaked))
    missing = missing_requirements(repo, harness, tasks)
    if missing:
        raise SystemExit('the harness revision lacks commits the tasks need: ' + '; '.join(missing))
    temp = shared_temp(output)
    if temp:
        raise SystemExit('--output must not lie in a shared temporary root: ' + ', '.join(temp))
    found = parent_instructions(output)
    if found:
        raise SystemExit('--output lies below Claude Code instructions that every arm would load: ' + ', '.join(found))
    if gates:
        check_gates(gates, tasks, oracles)
    elif not dry_run:
        raise SystemExit('--gates <gates.json from gate_c.py> is required for a real run')


def loadavg() -> list:
    return [round(value, 2) for value in os.getloadavg()]
