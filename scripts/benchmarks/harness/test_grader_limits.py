"""Every process of a sandboxed run is capped, and a cut stderr says so (Phase 4 review, items 6 and 8).

The seam is grader.run_sandboxed, the one launcher of oracle, warmup, calibration and surface driver
runs; a plain Python child reports what it inherited, so no sandbox is needed to observe the caps.
"""
import errno
import json
from pathlib import Path
import resource
import sys
import tempfile
import unittest

import grader

REPORT = '''import errno, json, resource, signal, sys
kinds = {'cpu': resource.RLIMIT_CPU, 'fsize': resource.RLIMIT_FSIZE, 'nproc': resource.RLIMIT_NPROC}
limits = {name: resource.getrlimit(kind) for name, kind in kinds.items()}
try:
    hard = resource.getrlimit(resource.RLIMIT_CPU)[1]
    resource.setrlimit(resource.RLIMIT_CPU, (hard + 1, hard + 1))
    raised = 'raised'
except (ValueError, OSError) as error:
    raised = type(error).__name__
signal.signal(signal.SIGXFSZ, signal.SIG_IGN)
try:
    with open(sys.argv[1], 'wb') as sink:
        sink.seek(int(sys.argv[2]))
        sink.write(b'x')
    big = 'written'
except OSError as error:
    big = errno.errorcode[error.errno]
print(json.dumps({'limits': limits, 'raise': raised, 'big': big}))
'''


class RunLimitTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.base = Path(directory.name).resolve()

    def run_child(self, argv, timeout=45):
        result = grader.run_sandboxed(argv, self.base, self.base / 'out', self.base / 'err', timeout)
        return result, (self.base / 'out').read_bytes(), (self.base / 'err').read_bytes()

    def test_cpu_file_size_and_process_caps_are_hard_limits_of_the_child(self):
        nproc_hard = resource.getrlimit(resource.RLIMIT_NPROC)[1]
        result, out, err = self.run_child([sys.executable, '-c', REPORT, str(self.base / 'big'),
                                           str(grader.FILE_SIZE_LIMIT)])
        self.assertEqual(result['exit_code'], 0, err)
        report = json.loads(out)
        self.assertEqual(report['limits']['cpu'], [90, 90], 'twice the 45 s wall timeout, soft = hard')
        self.assertEqual(report['limits']['fsize'], [1 << 30, 1 << 30])
        self.assertEqual(report['raise'], 'ValueError', 'the child cannot raise a cap back')
        self.assertEqual(report['big'], 'EFBIG', 'a write past 1 GiB fails')
        self.assertFalse((self.base / 'big').stat().st_size > grader.FILE_SIZE_LIMIT)
        soft, hard = report['limits']['nproc']
        if sys.platform == 'darwin':
            self.assertEqual(soft, hard)
            self.assertTrue(grader.PROCESS_HEADROOM < soft <= nproc_hard, soft)
        self.assertEqual(grader.run_limits(10)[resource.RLIMIT_CPU], 60, 'at least a minute of CPU')

    def test_a_cut_stderr_ends_with_the_stamp_and_a_whole_one_does_not(self):
        flood = 'import sys; sys.stderr.buffer.write(b"e" * (%d + 4096))' % grader.OUTPUT_LIMIT
        result, out, err = self.run_child([sys.executable, '-c', flood])
        self.assertEqual((result['exit_code'], result['overflow'], out), (0, False, b''))
        self.assertEqual(err, b'e' * grader.OUTPUT_LIMIT + grader.STDERR_STAMP)
        _, _, err = self.run_child([sys.executable, '-c', 'import sys; sys.stderr.write("short\\n")'])
        self.assertEqual(err, b'short\n')


if __name__ == '__main__':
    unittest.main()
