"""Bind native binaries and release artifacts to the exact shared-source snapshot."""
from pathlib import Path
import argparse, hashlib, json, sys

def digest(path):
    with Path(path).open('rb') as f:return hashlib.file_digest(f,'sha256').hexdigest()
def fingerprint(root,patterns=None):
    root=Path(root);rows={}
    for p in sorted(root.rglob('*')):
        if p.is_file() and (patterns is None or p.suffix in patterns):rows[p.relative_to(root).as_posix()]=digest(p)
    return hashlib.sha256(json.dumps(rows,sort_keys=True,separators=(',',':')).encode()).hexdigest()
def android_fingerprint(project):
    roots=['core','app/src/main','native','scripts']
    rows={}
    for name in roots:
        for p in sorted((project/name).rglob('*')):
            if p.is_file() and not any(x in p.parts for x in ('jniLibs','__pycache__')):rows[p.relative_to(project).as_posix()]=digest(p)
    for name in ('app/build.gradle.kts','build.gradle.kts','settings.gradle.kts','gradle.properties','build-core.ps1','release.json'):rows[name]=digest(project/name)
    return hashlib.sha256(json.dumps(rows,sort_keys=True,separators=(',',':')).encode()).hexdigest()
def record_native(project,abi):
    state=json.loads((project/'sync/state.json').read_text())
    lib=project/f'app/src/main/jniLibs/{abi}/libnawacore.so'
    data={'schema':1,'abi':abi,'coreFingerprint':fingerprint(project/'core',{'.go','.c','.mod','.sum'}),'sharedRevision':state['desktopRevision'],'sha256':digest(lib)}
    path=project/f'sync/native-{abi}.json';path.write_text(json.dumps(data,indent=2)+'\n');return data
def verify_native(project,abis):
    state=json.loads((project/'sync/state.json').read_text());core=fingerprint(project/'core',{'.go','.c','.mod','.sum'})
    for abi in abis:
        p=project/f'sync/native-{abi}.json'
        if not p.exists():raise RuntimeError(f'Build the {abi} Go core with build-core.ps1 first')
        data=json.loads(p.read_text());lib=project/f'app/src/main/jniLibs/{abi}/libnawacore.so'
        if data['coreFingerprint']!=core or data['sha256']!=digest(lib):raise RuntimeError(f'Stale {abi} native core; rerun build-core.ps1')
    return {'nativeVerified':abis,'sharedRevision':state['desktopRevision']}
def record_release(project,artifact):
    verify_native(project,['arm64-v8a']);state=json.loads((project/'sync/state.json').read_text());release=json.loads((project/'release.json').read_text())
    data={'schema':1,'version':release['version'],'versionCode':release['versionCode'],'sharedRevision':state['desktopRevision'],'androidFingerprint':android_fingerprint(project),'artifact':Path(artifact).name,'sha256':digest(artifact),'bytes':Path(artifact).stat().st_size}
    (project/'BUILD-SOURCE.json').write_text(json.dumps(data,indent=2)+'\n');return data
def verify_release(project,artifact):
    state=json.loads((project/'sync/state.json').read_text());proof=json.loads((project/'BUILD-SOURCE.json').read_text())
    if proof['sharedRevision']!=state['desktopRevision'] or proof['androidFingerprint']!=android_fingerprint(project) or proof['sha256']!=digest(artifact):raise RuntimeError('Android release is stale. Build the paired Android release before publishing the PC update.')
    return proof
if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--project',type=Path,default=Path(__file__).resolve().parents[1]);parser.add_argument('--record-native');parser.add_argument('--verify-native');parser.add_argument('--record-release',type=Path);parser.add_argument('--verify-release',type=Path);args=parser.parse_args()
    try:
        if args.record_native:r=record_native(args.project,args.record_native)
        elif args.verify_native:r=verify_native(args.project,args.verify_native.split(','))
        elif args.record_release:r=record_release(args.project,args.record_release)
        elif args.verify_release:r=verify_release(args.project,args.verify_release)
        else:parser.error('Choose one operation')
        print(json.dumps(r))
    except Exception as e:print(str(e),file=sys.stderr);sys.exit(1)
