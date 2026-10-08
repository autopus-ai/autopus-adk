"""Black-box oracle documents and the REQ-HR-08 judgement table (SPEC-HARNEVAL-003, T9 and T13).

Trusted runner code. A signed-lane agent task carries `oracle_mode: black_box` and a
`black_box_oracle` definition; its trial builds the agent-modified artifact, runs it, and lets the
trusted oracle harness (cmd/harneval-oracle) compare the run with the pinned expectations. This
module validates the definition, prepares the trial inputs, checks every expected output against
its pinned SHA-256, writes the harness_oracle_input.v1 bundle, strictly decodes the
harness_oracle_result.v1 document, and applies the ten-row table the signer re-applies to the same
record and oracle result bytes. No process is started here.
"""
import base64
import hashlib
import json
from pathlib import Path
import re
import signal as signals

from golden_protocol import NO_ORACLE, RECORD_SCHEMA
from observe import reject_json_constant, unique_object
from workspace import _relative

INPUT_SCHEMA = 'harness_oracle_input.v1'
RESULT_SCHEMA = 'harness_oracle_result.v1'
RESULT_FILE = 'oracle_result.json'
OUTPUT_LIMIT = 1 << 20
INPUT_LIMIT = 16 << 20
MAX_ASSERTIONS = 32
STAGES = ('setup', 'agent', 'build', 'run', 'oracle')
CHECKS = ('ok', 'link_rejected', 'too_large', 'not_checked')
KINDS = ('exit_code', 'stdout', 'file')
SETUP_SIGNALS = ('workspace_setup_failed', 'mutation_failed', 'warmup_failed')
# The closed black-box signal table: no literal check and no white-box grading signal exists here.
OUTCOMES = {**dict.fromkeys(SETUP_SIGNALS, 'error'),
            **dict.fromkeys(('agent_launch_failed', 'agent_timeout', 'agent_exit_nonzero', 'observation_failed',
                             'scope_violation', 'artifact_build_failed', 'oracle_harness_error', 'artifact_timeout',
                             'output_link_rejected', 'output_too_large', 'expectation_mismatch'), 'fail'),
            'accepted': 'pass'}
NOT_LAUNCHED = {'launched': False, 'exit_code': None, 'os_signal': None, 'timed_out': False}
SHA256 = re.compile(r'^[0-9a-f]{64}$')
ASSERTION_ID = re.compile(r'^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$')
PACKAGE = re.compile(r'^\./[A-Za-z0-9_][A-Za-z0-9_./-]*$')
FIXED_PATH = re.compile(r'^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$')


def _pinned(value, role: str) -> dict:
    if not isinstance(value, dict) or set(value) != {'path', 'sha256'} or not isinstance(value['path'], str) \
            or not isinstance(value['sha256'], str) or not SHA256.match(value['sha256']):
        raise ValueError(role + ' must be {path, sha256} with a lowercase 64-hex digest')
    return {'path': value['path'], 'sha256': value['sha256']}


def _assertion(item) -> dict:
    if not isinstance(item, dict) or not isinstance(item.get('id'), str) or not ASSERTION_ID.match(item['id']):
        raise ValueError('assertion id is malformed: ' + repr(item))
    kind = item.get('kind')
    keys = {'exit_code': {'id', 'kind', 'exit_code'}, 'stdout': {'id', 'kind', 'expected'},
            'file': {'id', 'kind', 'path', 'expected'}}.get(kind)
    if keys is None or set(item) != keys:
        raise ValueError('assertion ' + item['id'] + ' must be one of ' + ', '.join(KINDS) + ' with its own fields')
    if kind == 'exit_code':
        if type(item['exit_code']) is not int:
            raise ValueError('assertion ' + item['id'] + ' exit_code must be an integer')
        return dict(item)
    if kind == 'file' and (not isinstance(item['path'], str) or not FIXED_PATH.match(item['path'])
                           or any(part in ('.', '..') for part in item['path'].split('/'))):
        raise ValueError('assertion ' + item['id'] + ' path must be a clean relative output path')
    return {**item, 'expected': _pinned(item['expected'], 'assertion ' + item['id'] + ' expected')}


def definition(task: dict) -> dict | None:
    """The validated black_box_oracle of a task document, or None for a white-box task."""
    mode = task.get('oracle_mode', 'white_box')
    if mode == 'white_box' and 'black_box_oracle' not in task:
        return None
    oracle = task.get('black_box_oracle')
    if mode != 'black_box' or not isinstance(oracle, dict) or not {'build', 'command', 'inputs', 'assertions'} \
            <= set(oracle) <= {'build', 'command', 'inputs', 'assertions', 'stdin'}:
        raise ValueError('oracle_mode black_box needs black_box_oracle{build, command, inputs, assertions[, stdin]}')
    build, command = oracle['build'], oracle['command']
    if not isinstance(build, str) or not PACKAGE.match(build) or '..' in build.split('/'):
        raise ValueError('black_box_oracle.build must be a ./ package path')
    if not isinstance(command, list) or not command or command[0] != '{artifact}' or \
            not all(isinstance(arg, str) and '{artifact}' not in arg for arg in command[1:]):
        raise ValueError('black_box_oracle.command must start with {artifact} and name it once')
    inputs = [_pinned(item, 'input') for item in oracle['inputs']] if isinstance(oracle['inputs'], list) else None
    names = [Path(item['path']).name for item in inputs or []]
    if inputs is None or len(set(names)) != len(names):
        raise ValueError('black_box_oracle.inputs must be a list of {path, sha256} with distinct file names')
    stdin = oracle.get('stdin')
    if stdin is not None and stdin not in names:
        raise ValueError('black_box_oracle.stdin must name one input')
    items = oracle['assertions']
    if not isinstance(items, list) or not 0 < len(items) <= MAX_ASSERTIONS:
        raise ValueError('black_box_oracle.assertions needs 1 to %d assertions' % MAX_ASSERTIONS)
    assertions = [_assertion(item) for item in items]
    if len({item['id'] for item in assertions}) != len(assertions):
        raise ValueError('black_box_oracle assertion ids repeat')
    return {'build': build, 'command': list(command), 'inputs': inputs, 'stdin': stdin, 'assertions': assertions}


def _pinned_bytes(set_root: Path, pinned: dict, limit: int) -> bytes:
    """A committed fixture below the set root: no symlink on its path, a regular file, its pinned digest."""
    path = _relative(Path(set_root), pinned['path'])
    if path.is_symlink() or not path.is_file():
        raise ValueError(pinned['path'] + ' is not a regular file')
    with open(path, 'rb') as source:
        data = source.read(limit + 1)
    if len(data) > limit or hashlib.sha256(data).hexdigest() != pinned['sha256']:
        raise ValueError(pinned['path'] + ' does not match its pinned sha256')
    return data


def prepare_fixtures(oracle: dict, set_root: Path, input_dir: Path) -> dict:
    """Copy the inputs into the new trial input directory and check each copy, then read every expected
    output from the set root and check it; the expected outputs stay in memory, never in a trial tree."""
    Path(input_dir).mkdir(parents=True)
    for pinned in oracle['inputs']:
        copy = Path(input_dir) / Path(pinned['path']).name
        copy.write_bytes(_pinned_bytes(set_root, pinned, INPUT_LIMIT))
        if hashlib.sha256(copy.read_bytes()).hexdigest() != pinned['sha256']:
            raise ValueError('the input copy ' + copy.name + ' does not match its pinned sha256')
        copy.chmod(0o444)
    return {item['id']: _pinned_bytes(set_root, item['expected'], OUTPUT_LIMIT)
            for item in oracle['assertions'] if item['kind'] != 'exit_code'}


def artifact_args(oracle: dict, artifact: Path, input_dir: Path, output_root: Path) -> list:
    """The artifact argv: {artifact}, {input} and {output} replaced by the trial's canonical paths."""
    places = {'{input}': str(input_dir), '{output}': str(output_root)}
    args = []
    for arg in oracle['command'][1:]:
        for placeholder, value in places.items():
            arg = arg.replace(placeholder, value)
        args.append(arg)
    return [str(artifact), *args]


def bundle(task_id: str, oracle: dict, expected: dict, run: dict, stdout: bytes) -> bytes:
    """The harness_oracle_input.v1 stdin bundle: assertions from the main task definition, the checked
    expected outputs, the captured stdout and the artifact's exit status."""
    def encode(data: bytes) -> str:
        return base64.b64encode(data).decode()
    items = []
    for item in oracle['assertions']:
        entry = {'id': item['id'], 'kind': item['kind']}
        if item['kind'] == 'exit_code':
            entry['exit_code'] = item['exit_code']
        else:
            entry.update({'path': item['path']} if item['kind'] == 'file' else {})
            entry['expected'] = encode(expected[item['id']])
        items.append(entry)
    document = {'schema_version': INPUT_SCHEMA, 'task_id': task_id, 'artifact_exit': artifact_exit(run),
                'timed_out': bool(run['timed_out']), 'stdout': encode(stdout[:OUTPUT_LIMIT]),
                'stdout_overflow': bool(run['overflow']), 'assertions': items}
    return json.dumps(document, separators=(',', ':')).encode()


def artifact_exit(run: dict) -> int | None:
    """The exit status the oracle sees: only an artifact that exited on its own within its time has one."""
    code = run.get('exit_code')
    return code if isinstance(code, int) and code >= 0 and not run['timed_out'] else None


def decode_result(data: bytes | None, task_id: str) -> dict | None:
    """A strictly valid harness_oracle_result.v1 document of this task, or None: unknown or repeated
    keys, a wrong type, a timeout that was checked, or a check other than ok with assertions."""
    if data is None:
        return None
    try:
        document = json.loads(data, object_pairs_hook=unique_object, parse_constant=reject_json_constant)
    except (ValueError, RecursionError, UnicodeDecodeError):
        return None
    if not isinstance(document, dict) or set(document) != {'schema_version', 'task_id', 'output_check', 'assertions',
                                                            'artifact_exit', 'timed_out'}:
        return None
    exit_code, items = document['artifact_exit'], document['assertions']
    if document['schema_version'] != RESULT_SCHEMA or document['task_id'] != task_id or \
            document['output_check'] not in CHECKS or type(document['timed_out']) is not bool or \
            not (exit_code is None or type(exit_code) is int) or not isinstance(items, list):
        return None
    if any(not isinstance(item, dict) or set(item) != {'id', 'passed'} or not isinstance(item['id'], str)
           or type(item['passed']) is not bool for item in items) or len({item['id'] for item in items}) != len(items):
        return None
    if document['timed_out'] != (document['output_check'] == 'not_checked') or \
            (document['output_check'] != 'ok' and items):
        return None
    return document


def agent_failure(termination: dict) -> str | None:
    """Row 2: the agent process's own end, by launch, timeout, then exit status or signal."""
    if not termination['launched']:
        return 'agent_launch_failed'
    if termination['timed_out']:
        return 'agent_timeout'
    if termination['exit_code'] not in (0, None) or termination['os_signal']:
        return 'agent_exit_nonzero'
    return None


def derive(stage: str, signal: str | None, termination: dict, data: bytes | None, task_id: str,
           assertion_ids: list) -> tuple:
    """(outcome, signal, oracle) by the first matching REQ-HR-08 row; ValueError when no row matches.

    `signal` is the stage signal the runner observed (a setup failure, observation_failed or
    scope_violation) or None. `oracle` is filled apart from the row: build_failed when the build
    stage failed, ran only for a schema-valid, checked result naming exactly the task's assertions.
    """
    result = decode_result(data, task_id) if stage == 'oracle' else None
    checked = result is not None and result['output_check'] == 'ok'
    ran = checked and {item['id'] for item in result['assertions']} == set(assertion_ids)
    passed = sum(item['passed'] for item in result['assertions']) if ran else 0
    oracle = {**NO_ORACLE, 'ran': ran, 'build_failed': stage == 'build', 'expected_passed': passed,
              'expected_failed': len(assertion_ids) - passed if ran else 0}
    if stage == 'setup' and signal in SETUP_SIGNALS:
        return 'error', signal, oracle
    failed = agent_failure(termination) if stage in STAGES[1:] else None
    row = failed or ('observation_failed' if signal == 'observation_failed' and stage in STAGES[1:] else None)
    row = row or ('scope_violation' if signal == 'scope_violation' and stage == 'agent' else None)
    row = row or ('artifact_build_failed' if stage == 'build' else None)
    if row is None and stage == 'oracle':
        if result is None or (checked and not ran):
            row = 'oracle_harness_error'
        elif result['timed_out']:
            row = 'artifact_timeout'
        elif not checked:
            row = 'output_link_rejected' if result['output_check'] == 'link_rejected' else 'output_too_large'
        else:
            row = 'accepted' if passed == len(assertion_ids) else 'expectation_mismatch'
    if row is None:
        raise ValueError('no REQ-HR-08 row matches stage %r with signal %r' % (stage, signal))
    return OUTCOMES[row], row, oracle


def termination(run: dict | None, agent_signal: str | None) -> dict:
    """agent_termination{launched, exit_code, os_signal, timed_out} of the agent step. An agent that never
    started, or (001 rule) exited nonzero before its first event, did not launch; a negative Python
    returncode becomes the signal name and a null exit code."""
    if run is None or not run['launched'] or (agent_signal == 'agent_launch_failed' and not run['timed_out']):
        return dict(NOT_LAUNCHED)
    code = run['exit_code']
    if isinstance(code, int) and code < 0:
        try:
            name = signals.Signals(-code).name
        except ValueError:
            name = 'SIG%d' % -code
        return {'launched': True, 'exit_code': None, 'os_signal': name, 'timed_out': bool(run['timed_out'])}
    return {'launched': True, 'exit_code': code, 'os_signal': None, 'timed_out': bool(run['timed_out'])}


def record(session_id: str, attempt: dict, derived: tuple, duration_s: float, stage: str, ended: dict,
           oracle_sha256: str | None) -> dict:
    """harness_golden_live_record.v1 of a black-box trial: the 001 fields plus stage_reached,
    agent_termination and oracle_result_sha256 (null unless the oracle stage was reached)."""
    outcome, signal, oracle = derived
    return {'schema_version': RECORD_SCHEMA, 'session_id': session_id, 'task_id': attempt['task_id'],
            'arm': attempt['arm'], 'trial': attempt['trial'], 'outcome': outcome, 'signal': signal,
            'oracle': oracle, 'duration_s': round(max(duration_s, 0.0), 3), 'stage_reached': stage,
            'agent_termination': ended, 'oracle_result_sha256': oracle_sha256 if stage == 'oracle' else None}


def store_result(out: Path, data: bytes) -> str:
    """Keep oracle result bytes content-addressed in <session>/oracle_results/<sha256>.json."""
    digest = hashlib.sha256(data).hexdigest()
    directory = Path(out) / 'oracle_results'
    directory.mkdir(exist_ok=True)
    try:
        with open(directory / (digest + '.json'), 'xb') as sink:
            sink.write(data)
    except FileExistsError:
        pass
    return digest

