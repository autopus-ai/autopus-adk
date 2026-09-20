"""Frozen 3-arm single-agent benchmark; operational failures remain observations."""
import argparse
import hashlib
import itertools
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import time

from observe import parse_events
from report import build_report, to_markdown

ARMS=('native','reduced','current')


def schedule(tasks):
    orders=list(itertools.permutations(ARMS))
    return [(task,arm) for i,task in enumerate(tasks) for arm in orders[i%6]]


def digest(value):
    return hashlib.sha256(value).hexdigest()


def execute(args,cwd,out,err,seconds,env,stdin=None):
    started=time.monotonic()
    merged=os.environ.copy();merged.update(env)
    with out.open('wb') as stdout,err.open('wb') as stderr:
        p=subprocess.Popen(args,cwd=cwd,env=merged,stdin=subprocess.PIPE if stdin else subprocess.DEVNULL,stdout=stdout,stderr=stderr,start_new_session=True)
        timed_out=False
        try:p.communicate(stdin.encode() if stdin else None,timeout=seconds)
        except subprocess.TimeoutExpired:
            timed_out=True
            os.killpg(p.pid,signal.SIGTERM)
            try:p.wait(timeout=5)
            except subprocess.TimeoutExpired:os.killpg(p.pid,signal.SIGKILL);p.wait()
    return p.returncode,timed_out,round(time.monotonic()-started,3)


def copy_candidate(work, grade, allowed):
    valid=True
    for path in allowed:
        p=work/path
        if p.is_file() and not p.is_symlink():shutil.copyfile(p,grade/path)
        else:
            (grade/path).unlink(missing_ok=True)
            valid=False
    return valid


def main():
    from permissions import profile_args
    from workspace import snapshot,apply_mutation,install_surface,hashes,audit,initialize
    ap=argparse.ArgumentParser()
    ap.add_argument('--repo',type=Path,required=True)
    ap.add_argument('--output',type=Path,required=True)
    ap.add_argument('--surfaces',type=Path,required=True)
    ap.add_argument('--revision',required=True)
    ap.add_argument('--model',default='gpt-6-astra')
    ap.add_argument('--effort',default='medium')
    ap.add_argument('--timeout',type=int,default=180)
    ap.add_argument('--limit',type=int)
    args=ap.parse_args()
    base=Path(__file__).resolve().parent
    tasks=json.loads((base/'corpus_a.json').read_text())+json.loads((base/'corpus_b.json').read_text())
    tasks=sorted(tasks,key=lambda x:x['id'])
    if args.limit:tasks=tasks[:args.limit]
    out=args.output.resolve();out.mkdir(parents=True,exist_ok=True)
    if (out/'protocol.json').exists():raise SystemExit('refusing to overwrite frozen experiment')
    source=out/'source';snapshot(args.repo,args.revision,source)
    corpus_hash=digest(json.dumps(tasks,sort_keys=True).encode())
    protocol={'go_cache':'shared warmed local cache; fresh test execution uses -count=1','schema':'harness_benchmark.v1','revision':args.revision,'corpus_hash':corpus_hash,'tasks':tasks,'model':args.model,'effort':args.effort,'timeout_seconds':args.timeout,'order':[(t['id'],a) for t,a in schedule(tasks)],'cli_version':subprocess.check_output(['codex','--version'],text=True).strip(),'go_version':subprocess.check_output(['go','version'],text=True).strip(),'cache':'shared provider cache, order balanced; not cold-controlled','global_customization':'same account/global surfaces in all arms; ignore-user-config does not assert global-skill isolation','single_agent':True,'human_interventions':0,'arms':{}}
    for arm in ARMS:
        s=args.surfaces/arm
        skills=list((s/'.codex/skills').glob('*/SKILL.md'))
        files={str(p.relative_to(s)):digest(p.read_bytes()) for p in sorted(s.rglob('*')) if p.is_file()}
        protocol['arms'][arm]={'surface_hash':digest(json.dumps(files,sort_keys=True).encode()),'skills':len(skills),'manifest':files}
    (out/'protocol.json').write_text(json.dumps(protocol,indent=2))
    records=[]
    common_env={'GOMAXPROCS':'2','GOFLAGS':'-p=1','GOPROXY':'off','GOSUMDB':'off','PYTHONDONTWRITEBYTECODE':'1'}
    for index,(task,arm) in enumerate(schedule(tasks),1):
        name=task['id']+'-'+arm;trial=out/name;trial.mkdir()
        work=trial/'workspace';shutil.copytree(source,work)
        apply_mutation(work,task);install_surface(work,args.surfaces/arm);initialize(work)
        before=hashes(work)
        cache=out/'go-cache';cache.mkdir(exist_ok=True)
        temp=trial/'tmp';temp.mkdir()
        env={**common_env,'GOCACHE':str(cache),'GOTMPDIR':str(temp),'TMPDIR':str(temp)}
        # Warm each trial equally outside the measured model interval.
        warm=list(task['oracle']['command']);warm[warm.index('-run')+1]='^$'
        wrc,_,_=execute(warm,work,trial/'warm.log',trial/'warm.err',90,env)
        prompt=task['prompt']+'\n\nAllowed production files: '+', '.join(task['allowed_paths'])+'.\nUse a single agent; do not delegate. Fix the behavior and verify it. Read only this workspace and installed toolchain files; do not inspect parent directories, other trials, or benchmark implementation. Existing tests and harness files are immutable. You may add regression tests. Do not commit or use network. Finish without requesting clarification.\nFocused acceptance command: '+ ' '.join(task['oracle']['command'])
        cmd=['codex','exec','--ephemeral','--ignore-user-config','--disable','multi_agent','--disable','multi_agent_v2','--disable','memories','--disable','hooks','--disable','apps','--disable','browser_use','-m',args.model,'-c','model_reasoning_effort="'+args.effort+'"','-c','approval_policy="never"','--skip-git-repo-check','-C',str(work),'--json','-']
        cmd[2:2]=profile_args(work,cache,temp)
        print(json.dumps({'stage':'start','index':index,'total':len(tasks)*3,'task':task['id'],'arm':arm}),flush=True)
        rc,timed,elapsed=execute(cmd,work,trial/'events.jsonl',trial/'stderr.log',args.timeout,env,prompt)
        observation=parse_events(trial/'events.jsonl')
        scope=audit(before,work,task['allowed_paths'])
        # Grade a pristine source copy containing only allowed production edits.
        grade=trial/'grade';shutil.copytree(source,grade);apply_mutation(grade,task)
        if not copy_candidate(work,grade,task['allowed_paths']):scope['accepted_scope']=False
        grc,gtime,_=execute(task['oracle']['command'],grade,trial/'grade.log',trial/'grade.err',90,env)
        operational='warmup_failed' if wrc else ('agent_timeout' if timed else ('agent_exit' if rc else ('oracle_timeout' if gtime else None)))
        accepted=grc==0 and rc==0 and not timed and not observation['failed'] and scope['accepted_scope'] and wrc==0
        row={'task_id':task['id'],'arm':arm,'accepted':accepted,'timed_out':timed,'elapsed_seconds':elapsed,'observation':observation,'operational_error':operational,'oracle_exit':grc,'scope':scope,'human_corrections':0,'prompt_hash':digest(prompt.encode())}
        records.append(row);(trial/'result.json').write_text(json.dumps(row,indent=2))
        (out/'records.json').write_text(json.dumps(records,indent=2))
        report=build_report(records);(out/'report.json').write_text(json.dumps(report,indent=2));(out/'report.md').write_text(to_markdown(report))
        print(json.dumps({'stage':'done','index':index,'task':task['id'],'arm':arm,'accepted':accepted,'seconds':elapsed,'tokens':observation['total_tokens']}),flush=True)
        shutil.rmtree(grade)
    print('BENCHMARK_COMPLETE',flush=True)


if __name__=='__main__':main()
