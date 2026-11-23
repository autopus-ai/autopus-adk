"""Which Bash commands of a pilot C trial reach the network (possible contamination: the fixed heads are
public). Only commands that run are judged, never text: a Go import path such as
`github.com/insajin/autopus-adk/pkg/x` in a here-document, a sed expression or a quoted argument is
source text, not GitHub access.

A command is flagged when a simple command in it (after `;`, `&&`, `||`, `|`, newlines, subshells and
command substitutions; leading VAR=value assignments and wrappers such as env, sudo, time, nice, xargs
stripped) is a network tool (gh, curl, wget, nc, ssh, ...), a git subcommand that talks to a remote
(fetch, pull, push, ls-remote, remote update, submodule update, archive --remote, and clone unless its
source is a local path: agents clone scratch fixtures), a go command
that downloads modules (go get, go mod download, go install pkg@version), or an interpreter whose
inline or here-document program opens an http(s) URL. A here-document fed to a shell is read as
commands; one fed to anything else is data.
"""
import re
import shlex

NETWORK_TOOLS = {'gh', 'hub', 'curl', 'wget', 'nc', 'ncat', 'netcat', 'telnet', 'ssh', 'scp', 'sftp', 'rsync',
                 'aria2c', 'http', 'https', 'xh', 'httpie', 'ftp', 'socat'}
GIT_REMOTE = {'fetch', 'clone', 'pull', 'push', 'ls-remote'}
GIT_OPTIONS_WITH_VALUE = {'-C', '-c', '--git-dir', '--work-tree', '--namespace', '--exec-path', '--config-env'}
SHELLS = {'sh', 'bash', 'zsh', 'dash', 'ksh'}
INTERPRETERS = {'python', 'python3', 'node', 'ruby', 'perl', 'deno', 'bun'}
WRAPPERS = {'env', 'sudo', 'command', 'builtin', 'exec', 'time', 'nice', 'nohup', 'timeout', 'xargs', 'then',
            'do', 'else', 'elif', '!', '{', 'if', 'while', 'until'}
SEPARATORS = {';', '&&', '||', '|', '&', '(', ')', ';;', '|&', '\n'}
ASSIGNMENT = re.compile(r'^[A-Za-z_][A-Za-z0-9_]*=')
HEREDOC = re.compile(r'<<(-?)[ \t]*(["\']?)([A-Za-z_][A-Za-z0-9_]*)\2')
URL_FETCH = re.compile(r'https?://', re.I)
FETCH_API = re.compile(r'urlopen|urllib|requests\.|http\.client|httpx|fetch\(|https?\.get|net/http|'
                       r'socket\.|Net::HTTP|open-uri|LWP', re.I)
REMOTE_SOURCE = re.compile(r'^[a-z][a-z0-9+.-]*://|^[^/\s]+@[^/\s]+:|^[^/\s.][^/\s]*\.[a-z]{2,}:|github\.com', re.I)
SUBSTITUTION = re.compile(r'\$\(([^()]*(?:\([^()]*\)[^()]*)*)\)|`([^`]*)`')


def _split_heredocs(command: str) -> tuple:
    """(text without here-document bodies, [(line that opened it, body)])."""
    lines, kept, bodies, i = command.split('\n'), [], [], 0
    while i < len(lines):
        line = lines[i]
        kept.append(line)
        i += 1
        for match in HEREDOC.finditer(line):
            if line[max(0, match.start() - 1):match.start()] == '<':
                continue  # <<< is a here-string, not a here-document
            body = []
            while i < len(lines):
                end = lines[i].lstrip('\t') if match.group(1) else lines[i]
                i += 1
                if end == match.group(3):
                    break
                body.append(lines[i - 1])
            bodies.append((line, '\n'.join(body)))
    return '\n'.join(kept), bodies


def _tokens(text: str) -> list:
    lexer = shlex.shlex(text.replace('\n', ' ; '), posix=True, punctuation_chars=';&|()')
    lexer.whitespace_split = True
    lexer.commenters = '#'
    return list(lexer)


def _simple_commands(tokens: list) -> list:
    commands, current = [], []
    for token in tokens:
        if token in SEPARATORS or set(token) <= set(';&|()'):
            if current:
                commands.append(current)
            current = []
        else:
            current.append(token)
    return commands + ([current] if current else [])


def _strip_wrappers(words: list) -> list:
    i = 0
    while i < len(words):
        word = words[i].lstrip('`$(')
        if ASSIGNMENT.match(word) or word in WRAPPERS:
            i += 1
            # Wrapper options and their values (timeout 30, nice -n 5, env -u X, sudo -u user).
            while i < len(words) and (words[i].startswith('-') or words[i - 1] in ('timeout', '-n', '-u')
                                      and re.match(r'^[\w.]+$', words[i]) and words[i] not in NETWORK_TOOLS):
                i += 1
            continue
        break
    return words[i:]


def _remote_source(args: list) -> bool:
    """A clone source that is not a local path: a URL, scp-like host:path, or a hosting name."""
    operands = [a for a in args if not a.startswith('-')]
    return not operands or any(REMOTE_SOURCE.search(a) for a in operands)


def _judge(words: list) -> str | None:
    """Why one simple command reaches the network, or None."""
    words = _strip_wrappers(words)
    if not words:
        return None
    tool = words[0].lstrip('`$(').rsplit('/', 1)[-1]
    args = words[1:]
    if tool in NETWORK_TOOLS:
        return tool
    if tool == 'git':
        i = 0
        while i < len(args) and args[i].startswith('-'):
            i += 2 if args[i] in GIT_OPTIONS_WITH_VALUE else 1
        sub, rest = (args[i], args[i + 1:]) if i < len(args) else (None, [])
        if sub in GIT_REMOTE:
            return 'git ' + sub if sub != 'clone' or _remote_source(rest) else None
        if (sub == 'remote' and rest[:1] == ['update']) or \
                (sub == 'submodule' and 'update' in rest) or (sub == 'archive' and any(a.startswith('--remote') for a in rest)):
            return 'git ' + sub
        return None
    if tool == 'go' and args:
        if args[0] == 'get' or args[:2] == ['mod', 'download'] or (args[0] == 'install' and any('@' in a for a in args[1:])):
            return 'go ' + args[0]
        return None
    if tool in SHELLS:
        for flag, program in zip(args, args[1:]):
            if flag.startswith('-') and 'c' in flag:
                return network_reason(program)
        return None
    if tool in INTERPRETERS:
        code = ' '.join(args)
        if URL_FETCH.search(code) and FETCH_API.search(code):
            return tool + ' url'
    return None


def network_reason(command: str) -> str | None:
    """The first network access in a Bash command line, or None (see module doc)."""
    text, bodies = _split_heredocs(command)
    for opener, body in bodies:
        # Every simple command on the opening line: `cat <<EOF | bash` feeds the body to bash.
        heads = [_strip_wrappers(words)[:1] for words in _simple_commands(_tokens_or_empty(opener))]
        names = [head[0].lstrip('`$(').rsplit('/', 1)[-1] for head in heads if head]
        name = names[0] if names else ''
        if any(n in SHELLS for n in names):
            found = network_reason(body)
        elif name in INTERPRETERS:
            found = name + ' url' if URL_FETCH.search(body) and FETCH_API.search(body) else None
        else:
            found = None
        if found:
            return found
    for match in SUBSTITUTION.finditer(text):
        found = network_reason(match.group(1) or match.group(2) or '')
        if found:
            return found
    for words in _simple_commands(_tokens_or_empty(text)):
        found = _judge(words)
        if found:
            return found
    return None


def _tokens_or_empty(text: str) -> list:
    try:
        return _tokens(text)
    except ValueError:
        # An unbalanced quote: drop quoted spans so their text is never read as a command.
        return _tokens(re.sub(r'"[^"]*"|\'[^\']*\'', ' ', text).replace('"', ' ').replace("'", ' '))
