import importlib.util,json,tempfile,unittest
from pathlib import Path
spec=importlib.util.spec_from_file_location('shared_sync',Path(__file__).with_name('sync_desktop.py'));sync=importlib.util.module_from_spec(spec);spec.loader.exec_module(sync)
identity_spec=importlib.util.spec_from_file_location('build_identity',Path(__file__).with_name('build_identity.py'));identity=importlib.util.module_from_spec(identity_spec);identity_spec.loader.exec_module(identity)

class SharedReleaseTests(unittest.TestCase):
 def fixture(self):
  temporary=tempfile.TemporaryDirectory();self.addCleanup(temporary.cleanup);root=Path(temporary.name);desktop=root/'desktop';project=root/'android';(desktop/'frontend').mkdir(parents=True);(project/'app/src/main/assets').mkdir(parents=True);(project/'core').mkdir()
  files={'frontend/style.css':b'base theme\n','core.go':b'package main\nfunc clock(){old()}\n','go.mod':b'module test\n','go.sum':b''}
  for name,data in files.items():(desktop/name).write_bytes(data)
  for name,data in files.items():
   dest=project/('app/src/main/assets/'+name[9:] if name.startswith('frontend/') else 'core/'+name);dest.write_bytes(data)
  (project/'DESKTOP-SOURCE.json').write_text(json.dumps({'files':{'core.go':sync.sha(files['core.go'])}}));sync.bootstrap(project,desktop);return desktop,project
 def test_desktop_theme_propagates_without_overwriting_mobile_adapter(self):
  desktop,project=self.fixture();native=project/'app/src/main/assets/android.css';native.write_text('touch layouts')
  (desktop/'frontend/style.css').write_text('new shared theme\n');sync.sync(project,desktop,'apply')
  self.assertEqual((project/'app/src/main/assets/style.css').read_text(),'new shared theme\n');self.assertEqual(native.read_text(),'touch layouts');self.assertFalse(sync.sync(project,desktop)['changed'])
 def test_disjoint_long_line_edits_preserve_mobile_audio_and_new_desktop_fix(self):
  base=b'const audio=desktop(); function clock(){return oldTime;}\n'
  ours=base.replace(b'desktop()',b'androidBridge()');theirs=base.replace(b'oldTime',b'fixedTime')
  self.assertEqual(sync.merge(base,ours,theirs),b'const audio=androidBridge(); function clock(){return fixedTime;}\n')
 def test_conflict_leaves_all_assets_and_baseline_unchanged(self):
  desktop,project=self.fixture();(project/'core/core.go').write_text('package main\nfunc clock(){mobile()}\n');(desktop/'core.go').write_text('package main\nfunc clock(){different()}\n');(desktop/'frontend/style.css').write_text('new theme\n')
  state=(project/'sync/state.json').read_bytes()
  with self.assertRaises(RuntimeError):sync.sync(project,desktop,'apply')
  self.assertEqual((project/'app/src/main/assets/style.css').read_bytes(),b'base theme\n');self.assertEqual((project/'sync/state.json').read_bytes(),state)
 def test_new_native_signature_requires_port_review(self):
  desktop,project=self.fixture();(desktop/'core.go').write_text('package main\nfunc (s *Studio) PreparePlayback(id string, mode int) (Playback,error) { return Playback{},nil }\n')
  self.assertTrue(sync.sync(project,desktop)['conflicts'])
 def test_deleted_file_with_android_edits_is_preserved(self):
  desktop,project=self.fixture();(project/'core/core.go').write_text('Android adapter\n');(desktop/'core.go').unlink()
  with self.assertRaises(RuntimeError):sync.sync(project,desktop,'apply')
  self.assertTrue((project/'core/core.go').is_file())
 def test_modified_native_binary_or_go_source_cannot_pass_build_gate(self):
  desktop,project=self.fixture();lib=project/'app/src/main/jniLibs/arm64-v8a/libnawacore.so';lib.parent.mkdir(parents=True);lib.write_bytes(b'test ELF fixture')
  identity.record_native(project,'arm64-v8a');identity.verify_native(project,['arm64-v8a']);(project/'core/core.go').write_text('new clock fix')
  with self.assertRaises(RuntimeError):identity.verify_native(project,['arm64-v8a'])
 def test_index_and_audio_hooks_are_idempotent_under_new_shared_script(self):
  html=b'<head></head><body><script src="app.js"></script><script src="updates.js"></script></body>'
  output=sync.android_index(html).decode();self.assertEqual(output.count('android-bridge.js'),1);self.assertIn('updates.js',output);self.assertLess(output.index('android-bridge.js'),output.index('src="app.js"'))

if __name__=='__main__':unittest.main()
