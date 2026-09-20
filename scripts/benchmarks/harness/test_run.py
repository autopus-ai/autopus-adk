"""Runner scheduling and failure-preserving execution contracts."""
import tempfile
import unittest
from pathlib import Path
from run import schedule, execute, copy_candidate

class RunnerTests(unittest.TestCase):
    def test_balanced_order(self):
        plan=schedule([{'id':str(i)} for i in range(12)])
        self.assertEqual(len(plan),36)
        for pos in range(3):
            arms=[plan[i*3+pos][1] for i in range(12)]
            self.assertEqual({a:arms.count(a) for a in set(arms)}, {'native':4,'reduced':4,'current':4})

    def test_timeout_keeps_output(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d)
            rc,timeout,_=execute(['python3','-u','-c','import time;print("started");time.sleep(10)'],root,root/'out',root/'err',.1,{})
            self.assertTrue(timeout)
            self.assertNotEqual(rc,0)
            self.assertIn('started',(root/'out').read_text())

    def test_missing_candidate_cannot_restore_solution(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);work=root/'work';grade=root/'grade';work.mkdir();grade.mkdir()
            (grade/'source.go').write_text('seeded bug')
            self.assertFalse(copy_candidate(work,grade,['source.go']))
            self.assertFalse((grade/'source.go').exists())
            (work/'source.go').symlink_to('/dev/null')
            (grade/'source.go').write_text('seeded bug')
            self.assertFalse(copy_candidate(work,grade,['source.go']))
            self.assertFalse((grade/'source.go').exists())

if __name__=='__main__':unittest.main()
