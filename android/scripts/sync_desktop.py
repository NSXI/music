"""Three-way source synchronization. Never reads the user's library or tools."""
from __future__ import annotations
import argparse, difflib, hashlib, json, os, re, sys
from pathlib import Path

SKIP = {"main.go", "fullscreen.go"}
WATCH = {"auth_windows.go", "worker_resources_windows.go"}

def sha(data): return hashlib.sha256(data).hexdigest()
def canonical(data): return data.decode("utf-8-sig").replace("\r\n", "\n")
def edits(base, target):
    return [(a,b,target[c:d]) for op,a,b,c,d in difflib.SequenceMatcher(None,base,target,autojunk=False).get_opcodes() if op!="equal"]
def intersects(a,b):
    x,y,_=a; u,v,_=b
    if x==y and u==v: return x==u
    if x==y: return u<=x<v
    if u==v: return x<=u<y
    return x<v and u<y
def merge_units(base, ours, theirs):
    local=edits(base,ours); remote=edits(base,theirs); combined=list(local)
    for item in remote:
        if item in local: continue
        if any(intersects(item,other) for other in local): return None
        combined.append(item)
    result=[]; cursor=0
    for a,b,replacement in sorted(combined,key=lambda e:(e[0],e[1])):
        if a<cursor:return None
        result.extend(base[cursor:a]);result.extend(replacement);cursor=b
    result.extend(base[cursor:]);return "".join(result)
def merge(base:bytes,ours:bytes,theirs:bytes):
    if ours==theirs or theirs==base:return ours
    if ours==base:return theirs
    b,o,t=map(canonical,(base,ours,theirs))
    result=merge_units(b.splitlines(keepends=True),o.splitlines(keepends=True),t.splitlines(keepends=True))
    if result is None:
        # Desktop JS has long lines. Preserve all token text, and merge only
        # disjoint edits; ambiguity still requires an explicit port.
        token=lambda s:re.findall(r"\w+|\s+|[^\w\s]",s,re.UNICODE)
        result=merge_units(token(b),token(o),token(t))
    return None if result is None else result.encode("utf-8")

def shared_files(desktop):
    files={}
    for p in (desktop/"frontend").rglob("*"):
        if p.is_file():files["assets/"+p.relative_to(desktop/"frontend").as_posix()]=p.read_bytes()
    for p in desktop.glob("*.go"):
        if p.name in SKIP or p.name.endswith(("_test.go","_windows.go")):continue
        files["core/"+p.name]=p.read_bytes()
    for name in ("go.mod","go.sum"):
        files["core/"+name]=(desktop/name).read_bytes()
    if (desktop/"release.json").is_file():files["release.json"]=(desktop/"release.json").read_bytes()
    return files
def destination(project,key):
    if key.startswith("assets/"):return project/"app/src/main/assets"/key[7:]
    return project/key
def prepare_new(key,data):
    if key=="assets/index.html":return android_index(data)
    if key=="assets/app.js":return android_app(data)
    if key.startswith("core/") and key.endswith(".go"):
        data=data.replace(b'"github.com/wailsapp/wails/v2/pkg/runtime"',b'"nawa.android/core/desktop"')
    return data
def android_index(data):
    text=canonical(data)
    checks=("</head>",'<script src="app.js">',"</body>")
    if not all(text.count(s)==1 for s in checks):raise RuntimeError("Desktop HTML entrypoints changed; update Android template adapter")
    text=text.replace("width=device-width, initial-scale=1","width=device-width, initial-scale=1, viewport-fit=cover")
    text=text.replace("</head>",'<link rel="stylesheet" href="android.css"></head>').replace('<script src="app.js">','<script src="android-bridge.js"></script><script src="app.js">').replace("</body>",'<script src="android-ui.js"></script></body>')
    return text.encode("utf8")
def android_app(data):
    text=canonical(data)
    substitutions={
      "function ensureGraph(){":"function ensureGraph(){if(window.NawaMobile)return graph||(graph={ctx:{resume:()=>Promise.resolve()}});",
      "function applyFX(){":"function applyFX(){if(window.NawaMobile){window.NawaMobile.fx(ui);persist(true);return;}",
      "function drawSpectrum(){":"function drawSpectrum(){if(window.NawaMobile)return;",
      "audio.addEventListener('ended',()=>{":"audio.addEventListener('ended',()=>{if(window.NawaMobile)return;",
      "async function poll(){try{":"async function poll(){if(window.NawaMobile&&document.body.classList.contains('android-suspended'))return;try{",
      "setInterval(poll,1000)":"setInterval(()=>window.NawaMobile.poll(),1000)",
    }
    for before,after in substitutions.items():
        if text.count(before)!=1:raise RuntimeError("Android audio/clock adapter needs review: "+before)
        text=text.replace(before,after)
    return text.replace("محفوظًا بحماية Windows","محفوظًا بحماية Android Keystore").replace("مشفرًا بحماية Windows","مشفرًا بحماية Android Keystore").encode("utf8")
def tree_hash(files):return sha(json.dumps({k:sha(v) for k,v in sorted(files.items())},sort_keys=True,separators=(",",":")).encode())
def contracts(data):
    result={}
    for key,value in data.items():
        if key.startswith("core/") and key.endswith(".go"):
            for m in re.finditer(r"func\s+\(\w+\s+\*Studio\)\s+([A-Z]\w*)\s*\(([^)]*)\)\s*([^\n{]*)",canonical(value)):
                result[m[1]]=re.sub(r"\s+"," ",m[2]+" -> "+m[3]).strip()
    return result
NATIVE_CONTRACTS={"PreparePlayback","DecodePlayback","PreparePreview","ImportFiles","AddLibraryFolder","RescanLibraryFolders","LoadLyricsFile","GetFonts","Window","SpatialSettings","ChooseFolder","SetLyricsFullscreen","OpenURL"}

def sync(project,desktop,mode="check",accept_platform=False):
    project=Path(project).resolve();desktop=Path(desktop).resolve();control=project/"sync";state_file=control/"state.json"
    if not desktop.is_dir():
        if not state_file.is_file():raise RuntimeError("Desktop source and shared-source snapshot are missing")
        return verify_snapshot(project)
    current=shared_files(desktop)
    if not state_file.exists():raise RuntimeError("Initialize with --bootstrap before changing the desktop source")
    state=json.loads(state_file.read_text(encoding="utf8"));old={k:(control/"desktop-base"/k).read_bytes() for k in state["files"]}
    conflicts=[];writes={};deletions=[]
    for key in sorted(set(old)|set(current)):
        path=destination(project,key);ours=path.read_bytes() if path.is_file() else None
        base=old.get(key);theirs=current.get(key)
        if base is None:
            result=prepare_new(key,theirs)
            if ours is not None and ours!=result:conflicts.append({"file":key,"reason":"new desktop file collides with Android file"});continue
        elif theirs is None:
            if ours is not None and ours!=base:conflicts.append({"file":key,"reason":"desktop deleted a file with Android adaptations"});continue
            if ours is not None:deletions.append(path)
            continue
        elif ours is None:
            if theirs!=base:conflicts.append({"file":key,"reason":"Android file missing while desktop changed"})
            continue
        elif key in {"assets/index.html","assets/app.js"}:
            if canonical(ours)!=canonical(prepare_new(key,base)):
                conflicts.append({"file":key,"reason":"unrecorded edit in generated Android platform entrypoint"});continue
            result=prepare_new(key,theirs)
        else:result=merge(base,ours,theirs)
        if result is None:conflicts.append({"file":key,"reason":"overlapping platform and desktop edits"})
        elif result!=ours:writes[path]=result
    new_contracts=contracts(current);old_contracts=contracts(old)
    for name in NATIVE_CONTRACTS:
        if old_contracts.get(name)!=new_contracts.get(name) and not accept_platform:
            conflicts.append({"file":name,"reason":"native bridge signature changed; port and test Android before --accept-platform"})
    watch={name:sha((desktop/name).read_bytes()) for name in WATCH if (desktop/name).is_file()}
    if watch!=state.get("platformWatch",{}) and not accept_platform:conflicts.append({"file":"Windows platform adapters","reason":"native authentication/resource contract changed; review Android adaptation"})
    report={"schema":1,"desktopRevision":tree_hash(current),"changed":[str(p.relative_to(project)).replace("\\","/") for p in writes],"deleted":[str(p.relative_to(project)).replace("\\","/") for p in deletions],"conflicts":conflicts,"mode":mode}
    control.mkdir(parents=True,exist_ok=True)
    if mode=="apply":
        (control/"last-report.json").write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n",encoding="utf8")
        if conflicts:raise RuntimeError("Shared-source synchronization requires a port: "+json.dumps(conflicts,ensure_ascii=False))
        for path,data in writes.items():
            path.parent.mkdir(parents=True,exist_ok=True);temporary=path.with_name(path.name+".sync-temp");temporary.write_bytes(data);os.replace(temporary,path)
        for path in deletions:path.unlink()
        for key,data in current.items():
            path=control/"desktop-base"/key;path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(data)
        state={"schema":1,"sourceHint":"../Madar-Music","desktopRevision":tree_hash(current),"files":{k:sha(v) for k,v in sorted(current.items())},"androidFiles":{k:sha(destination(project,k).read_bytes()) for k in current if destination(project,k).is_file()},"platformWatch":watch}
        state_file.write_text(json.dumps(state,indent=2)+"\n",encoding="utf8")
    return report

def bootstrap(project,desktop):
    control=project/"sync";state_file=control/"state.json"
    if state_file.exists():raise RuntimeError("Snapshot already initialized; use --apply")
    original=json.loads((project/"DESKTOP-SOURCE.json").read_text(encoding="utf8"))
    for name,digest in original["files"].items():
        if sha((desktop/name).read_bytes())!=digest:raise RuntimeError("Original desktop baseline changed: "+name)
    files=shared_files(desktop)
    for key,data in files.items():
        dest=destination(project,key)
        if key.startswith("assets/") and key not in {"assets/app.js","assets/index.html"} and dest.read_bytes()!=data:raise RuntimeError("Unknown initial asset adaptation: "+key)
        p=control/"desktop-base"/key;p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(data)
    state={"schema":1,"desktopRevision":tree_hash(files),"files":{k:sha(v) for k,v in files.items()},"androidFiles":{k:sha(destination(project,k).read_bytes()) for k in files if destination(project,k).is_file()},"platformWatch":{n:sha((desktop/n).read_bytes()) for n in WATCH if (desktop/n).is_file()}}
    state_file.write_text(json.dumps(state,indent=2)+"\n",encoding="utf8")
    return {"initialized":len(files),"desktopRevision":state["desktopRevision"]}
def verify_snapshot(project):
    state=json.loads((project/"sync/state.json").read_text(encoding="utf8"));changed=[]
    for key,digest in state["androidFiles"].items():
        p=destination(project,key)
        if not p.is_file() or sha(p.read_bytes())!=digest:changed.append(key)
    if changed:raise RuntimeError("Shared snapshot has unrecorded edits; sync with desktop before release: "+", ".join(changed))
    return {"snapshotVerified":True,"desktopRevision":state["desktopRevision"]}

if __name__=="__main__":
    cli=argparse.ArgumentParser();cli.add_argument("--desktop",type=Path);cli.add_argument("--project",type=Path);cli.add_argument("--bootstrap",action="store_true");cli.add_argument("--apply",action="store_true");cli.add_argument("--check",action="store_true");cli.add_argument("--verify",action="store_true");cli.add_argument("--accept-platform",action="store_true");args=cli.parse_args()
    project=(args.project or Path(__file__).resolve().parents[1]).resolve();desktop=(args.desktop or Path(os.environ.get('NAWA_DESKTOP_SOURCE',project.parent/('desktop' if (project.parent/'desktop').is_dir() else 'Madar-Music')))).resolve()
    try:
        if args.bootstrap:result=bootstrap(project,desktop)
        elif args.verify:result=verify_snapshot(project)
        else:result=sync(project,desktop,"apply" if args.apply else "check",args.accept_platform)
        print(json.dumps(result,ensure_ascii=False))
        if args.check and (result.get("changed") or result.get("deleted") or result.get("conflicts")):sys.exit(2)
    except Exception as error:print(str(error),file=sys.stderr);sys.exit(1)
