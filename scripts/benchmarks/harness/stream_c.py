"""Observation of one Claude Code stream-json transcript (pilot C).

With --include-hook-events every hook emits `system` `hook_started` and `hook_response` events
carrying hook_event, exit_code, stdout, stderr and outcome. A Stop response is recorded as an
outcome (exit code, decision, systemMessage such as a verify-stop budget or skip note) and as a block
when it exits 2 or prints {"decision": "block"}. Bash commands that reach GitHub or fetch from the
network are flagged as possible contamination: the fixed heads are public. Only commands that run count
(network_c), so a Go import path in a here-document or a sed edit is not GitHub access; a web tool use
(WebFetch, WebSearch, disallowed in every arm) is flagged too, and so is a web search marker in the output
of a Bash call that ran codex (codex_c: a cross-model review step; its web search is configured off). Bash commands that set
GOCACHE, TMPDIR, CGO_ENABLED, GOMODCACHE or GOFLAGS are recorded as env_overrides, a friction signal:
an agent that replaces the trial's environment is working around it (and leaves the warm cache).
"""
import json
from pathlib import Path
import re

from codex_c import web_search_in_output

from network_c import network_reason
from observe import count, reject_json_constant, unique_object
from trial_env_c import env_overrides

MAX_LINE_BYTES = 8 * 1024 * 1024
WEB_TOOLS = ('WebFetch', 'WebSearch')
CODEX_COMMAND = re.compile(r'(^|[\s;&|(/`])codex(\s|$)')
USAGE_KEYS = ('input_tokens', 'output_tokens', 'cache_read_input_tokens', 'cache_creation_input_tokens')


def _stop_outcome(event: dict) -> dict:
    stdout = event.get('stdout') if isinstance(event.get('stdout'), str) else ''
    stderr = event.get('stderr') if isinstance(event.get('stderr'), str) else ''
    try:
        decision = json.loads(stdout.strip() or 'null')
    except ValueError:
        decision = None
    decision = decision if isinstance(decision, dict) else {}
    blocked = event.get('exit_code') == 2 or decision.get('decision') == 'block'
    reason = stderr if event.get('exit_code') == 2 else str(decision.get('reason', ''))
    return {'hook': event.get('hook_name'), 'exit_code': event.get('exit_code'), 'outcome': event.get('outcome'),
            'blocked': blocked, 'reason': reason[:2000] if blocked else None,
            'system_message': str(decision['systemMessage'])[:2000] if 'systemMessage' in decision else None}


def empty() -> dict:
    return {'schema_version': 'claude_stream_observation.v3', 'result_seen': False, 'is_error': None,
            'terminal_reason': None, 'num_turns': None, 'duration_ms': None, 'total_cost_usd': None,
            **{key: None for key in USAGE_KEYS}, 'total_tokens': None, 'model': None, 'permission_mode': None,
            'tool_uses': 0, 'subagent_tool_uses': 0, 'hook_events': 0, 'stop_hook_runs': 0, 'stop_outcomes': [],
            'stop_hook_blocks': [], 'contamination': [], 'env_overrides': [], 'final_text': None, 'incomplete_reasons': [],
            'codex_calls': 0}


def parse_stream(path: Path) -> dict:
    """Unknown usage stays None, never zero."""
    result, reasons, codex_ids = empty(), set(), set()
    try:
        source = Path(path).open('rb')
    except FileNotFoundError:
        result['incomplete_reasons'] = ['no_transcript']
        return result
    with source:
        for line in iter(lambda: source.readline(MAX_LINE_BYTES + 1), b''):
            if len(line) > MAX_LINE_BYTES or not line.endswith(b'\n'):
                reasons.add('truncated_or_oversized_line')
                continue
            try:
                event = json.loads(line, object_pairs_hook=unique_object, parse_constant=reject_json_constant)
            except (ValueError, UnicodeDecodeError, RecursionError):
                reasons.add('malformed_json')
                continue
            if not isinstance(event, dict):
                reasons.add('invalid_event_schema')
                continue
            _observe(event, result, codex_ids)
    if not result['result_seen']:
        reasons.add('no_result_event')
    result['incomplete_reasons'] = sorted(reasons | set(result['incomplete_reasons']))
    return result


def _observe(event: dict, result: dict, codex_ids: set) -> None:
    kind, subtype = event.get('type'), event.get('subtype')
    if kind == 'system' and subtype == 'init':
        result['model'], result['permission_mode'] = event.get('model'), event.get('permissionMode')
    elif kind == 'system' and subtype in ('hook_started', 'hook_response'):
        result['hook_events'] += 1
        if subtype == 'hook_response' and event.get('hook_event') == 'Stop':
            outcome = _stop_outcome(event)
            result['stop_hook_runs'] += 1
            result['stop_outcomes'].append(outcome)
            if outcome['blocked']:
                result['stop_hook_blocks'].append({'hook': outcome['hook'], 'reason': outcome['reason']})
    elif kind == 'assistant':
        content = (event.get('message') or {}).get('content') or []
        uses = [c for c in content if isinstance(c, dict) and c.get('type') == 'tool_use']
        result['tool_uses'] += len(uses)
        result['subagent_tool_uses'] += sum(1 for c in uses if c.get('name') in ('Task', 'Agent'))
        for use in uses:
            if use.get('name') in WEB_TOOLS:
                result['contamination'].append(str(use.get('name')) + ' ' + json.dumps(use.get('input'))[:400])
            command = (use.get('input') or {}).get('command') if isinstance(use.get('input'), dict) else None
            if use.get('name') != 'Bash' or not isinstance(command, str):
                continue
            if CODEX_COMMAND.search(command):
                result['codex_calls'] += 1
                codex_ids.add(use.get('id'))
            try:
                reached = network_reason(command)
            except ValueError:
                # Never lose a trial's observation to one unparseable line; flag it for a person.
                reached = 'unparsed'
            if reached:
                result['contamination'].append(('UNPARSED ' if reached == 'unparsed' else '') + command[:500])
            overridden = env_overrides(command)
            if overridden:
                result['env_overrides'].append({'vars': overridden, 'command': command[:500]})
    elif kind == 'user':
        _observe_codex_output(event, result, codex_ids)
    elif kind == 'result':
        _observe_result(event, result)


def _text(content) -> str:
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return '\n'.join(_text(item.get('text') if isinstance(item, dict) else item) for item in content)
    return ''


def _observe_codex_output(event: dict, result: dict, codex_ids: set) -> None:
    content = (event.get('message') or {}).get('content')
    for block in content if isinstance(content, list) else []:
        if not isinstance(block, dict) or block.get('type') != 'tool_result':
            continue
        if block.get('tool_use_id') in codex_ids and web_search_in_output(_text(block.get('content'))):
            result['contamination'].append('CODEX_WEB_SEARCH in output of tool use ' + str(block.get('tool_use_id')))


def _observe_result(event: dict, result: dict) -> None:
    result['result_seen'] = True
    result['is_error'] = event.get('is_error') if isinstance(event.get('is_error'), bool) else None
    for key in ('terminal_reason', 'num_turns', 'duration_ms', 'total_cost_usd'):
        result[key] = event.get(key)
    if isinstance(event.get('result'), str):
        result['final_text'] = event['result'][:4000]
    usage = event.get('usage')
    if not isinstance(usage, dict):
        result['incomplete_reasons'].append('missing_usage')
        return
    if not all(count(usage.get(key)) for key in USAGE_KEYS):
        result['incomplete_reasons'].append('invalid_usage')
        return
    for key in USAGE_KEYS:
        result[key] = usage[key]
    result['total_tokens'] = sum(usage[key] for key in USAGE_KEYS)


def arm_valid(arm: str, observation: dict) -> bool:
    """full must show hook events; native and instructions must show none."""
    return observation['hook_events'] > 0 if arm == 'full' else observation['hook_events'] == 0
