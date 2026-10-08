"""The black_box_oracle task definition the trusted runner accepts (SPEC-HARNEVAL-003 REQ-HR-08, T15).

The same rules as Go's BlackBoxOracle.validate (pkg/harneval/oracle_schema.go): a ./ package build, an
argv starting with {artifact}, pinned inputs with distinct file names, 1 to 32 assertions of the kinds
exit_code, stdout and file, and an optional positive control. A positive control is a second run of
the same artifact with the same command over its own pinned inputs that must exit 0 and print the
pinned stdout; the oracle harness reports it as the two CONTROL_IDS after the task's own assertions,
so a fix that refuses every input fails a task whose own assertions expect a refusal.
"""
from pathlib import Path
import posixpath
import re

MAX_ASSERTIONS = 32
ORACLE_ROOT = 'evals/harness/oracles'
KINDS = ('exit_code', 'stdout', 'file')
CONTROL_PREFIX = 'positive_control.'
CONTROL_IDS = (CONTROL_PREFIX + 'exit', CONTROL_PREFIX + 'stdout')
SHA256 = re.compile(r'^[0-9a-f]{64}$')
ASSERTION_ID = re.compile(r'^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$')
PACKAGE = re.compile(r'^\./[A-Za-z0-9_][A-Za-z0-9_./-]*$')
FIXED_PATH = re.compile(r'^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$')


def _pinned(value, role: str) -> dict:
    """One {path, sha256} pin: a clean path below ORACLE_ROOT, as Go's PinnedFile.validate requires."""
    if not isinstance(value, dict) or set(value) != {'path', 'sha256'} or not isinstance(value['path'], str) \
            or not isinstance(value['sha256'], str) or not SHA256.match(value['sha256']):
        raise ValueError(role + ' must be {path, sha256} with a lowercase 64-hex digest')
    name = value['path']
    if any(char in name for char in '\\\x00') or posixpath.normpath(name) != name or \
            not name.startswith(ORACLE_ROOT + '/'):
        raise ValueError(role + ' ' + repr(name) + ' is not a clean path under ' + ORACLE_ROOT + '/')
    return {'path': name, 'sha256': value['sha256']}


def _input_set(items, stdin, role: str) -> list:
    """Pinned inputs copied into one directory under their file names, so the names are distinct, and a
    stdin naming one of them."""
    inputs = [_pinned(item, role) for item in items] if isinstance(items, list) else None
    names = [Path(item['path']).name for item in inputs or []]
    if inputs is None or len(set(names)) != len(names):
        raise ValueError(role + 's must be a list of {path, sha256} with distinct file names')
    if stdin is not None and stdin not in names:
        raise ValueError(role + ' stdin must name one of them')
    return inputs


def _assertion(item) -> dict:
    if not isinstance(item, dict) or not isinstance(item.get('id'), str) or not ASSERTION_ID.match(item['id']) \
            or item['id'].startswith(CONTROL_PREFIX):
        raise ValueError('assertion id is malformed or takes the reserved ' + CONTROL_PREFIX + ' prefix: ' + repr(item))
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


def _positive_control(value) -> dict | None:
    if value is None:
        return None
    if not isinstance(value, dict) or not {'inputs', 'stdout'} <= set(value) <= {'inputs', 'stdin', 'stdout'} \
            or not isinstance(value['inputs'], list) or not value['inputs']:
        raise ValueError('black_box_oracle.positive_control needs {inputs (at least one), stdout[, stdin]}')
    return {'inputs': _input_set(value['inputs'], value.get('stdin'), 'positive_control input'),
            'stdin': value.get('stdin'), 'stdout': _pinned(value['stdout'], 'positive_control stdout')}


def definition(task: dict) -> dict | None:
    """The validated black_box_oracle of a task document, or None for a white-box task."""
    mode = task.get('oracle_mode', 'white_box')
    if mode == 'white_box' and 'black_box_oracle' not in task:
        return None
    oracle = task.get('black_box_oracle')
    if mode != 'black_box' or not isinstance(oracle, dict) or not {'build', 'command', 'inputs', 'assertions'} \
            <= set(oracle) <= {'build', 'command', 'inputs', 'assertions', 'stdin', 'positive_control'}:
        raise ValueError('oracle_mode black_box needs black_box_oracle{build, command, inputs, assertions'
                         '[, stdin, positive_control]}')
    build, command = oracle['build'], oracle['command']
    if not isinstance(build, str) or not PACKAGE.match(build) or '..' in build.split('/'):
        raise ValueError('black_box_oracle.build must be a ./ package path')
    if not isinstance(command, list) or not command or command[0] != '{artifact}' or \
            not all(isinstance(arg, str) and '{artifact}' not in arg for arg in command[1:]):
        raise ValueError('black_box_oracle.command must start with {artifact} and name it once')
    inputs = _input_set(oracle['inputs'], oracle.get('stdin'), 'black_box_oracle.input')
    items = oracle['assertions']
    if not isinstance(items, list) or not 0 < len(items) <= MAX_ASSERTIONS:
        raise ValueError('black_box_oracle.assertions needs 1 to %d assertions' % MAX_ASSERTIONS)
    assertions = [_assertion(item) for item in items]
    if len({item['id'] for item in assertions}) != len(assertions):
        raise ValueError('black_box_oracle assertion ids repeat')
    return {'build': build, 'command': list(command), 'inputs': inputs, 'stdin': oracle.get('stdin'),
            'assertions': assertions, 'positive_control': _positive_control(oracle.get('positive_control'))}


def assertion_ids(oracle: dict) -> list:
    """The trusted assertion ids of a definition in report order: its own, then the positive control's."""
    return [item['id'] for item in oracle['assertions']] + (list(CONTROL_IDS) if oracle.get('positive_control') else [])


def control_input(input_dir: Path) -> Path:
    """The positive control's own trial input directory, beside the task's."""
    return Path(input_dir).with_name(Path(input_dir).name + '-control')
