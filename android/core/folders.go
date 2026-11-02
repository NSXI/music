package main

import (
	"context"
	"errors"
	wr "nawa.android/core/desktop"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type FolderImportState struct {
	Running   bool     `json:"running"`
	Scanning  bool     `json:"scanning"`
	Cancelled bool     `json:"cancelled"`
	Total     int      `json:"total"`
	Processed int      `json:"processed"`
	Added     int      `json:"added"`
	Skipped   int      `json:"skipped"`
	Failed    int      `json:"failed"`
	Current   string   `json:"current"`
	Errors    []string `json:"errors,omitempty"`
}

var audioFileExtensions = map[string]bool{".flac": true, ".mp3": true, ".m4a": true, ".aac": true, ".wav": true, ".ogg": true, ".opus": true, ".aiff": true, ".aif": true, ".wma": true, ".ape": true, ".wv": true, ".dsf": true, ".dff": true, ".ac3": true, ".eac3": true, ".mka": true, ".mp4": true, ".alac": true, ".tta": true, ".webm": true, ".amr": true, ".m4b": true, ".tak": true}

func folderPathKey(path string) string { return strings.ToLower(filepath.Clean(path)) }
func (s *Studio) AddLibraryFolder() (FolderImportState, error) {
	if s.ctx == nil {
		return FolderImportState{}, errors.New("افتح التطبيق لاختيار مجلد")
	}
	path, err := wr.OpenDirectoryDialog(s.ctx, wr.OpenDialogOptions{Title: "Add music folder / إضافة مجلد موسيقى"})
	if err != nil {
		return FolderImportState{}, err
	}
	if path == "" {
		return s.GetState().FolderImport, nil
	}
	return s.ImportLibraryFolders([]string{path})
}
func (s *Studio) ImportLibraryFolders(paths []string) (FolderImportState, error) {
	if len(paths) == 0 || len(paths) > 64 {
		return FolderImportState{}, errors.New("اختر من مجلد واحد إلى 64 مجلدًا")
	}
	roots := []string{}
	seen := map[string]bool{}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			return FolderImportState{}, errors.New("اختر مسار مجلد غير فارغ")
		}
		abs, err := filepath.Abs(strings.TrimSpace(path))
		if err != nil {
			return FolderImportState{}, errors.New("مسار مجلد غير صالح")
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return FolderImportState{}, errors.New("المجلد غير موجود أو غير متاح")
		}
		identity := folderPathKey(abs)
		if !seen[identity] {
			roots = append(roots, abs)
			seen[identity] = true
		}
	}
	s.mu.Lock()
	if s.folderImport.Running {
		s.mu.Unlock()
		return FolderImportState{}, errors.New("يوجد فحص مجلدات قيد التشغيل")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.folderCancel = cancel
	s.folderImport = FolderImportState{Running: true, Scanning: true, Errors: []string{}}
	existing := map[string]bool{}
	for _, path := range s.state.Preferences.LibraryFolders {
		existing[folderPathKey(path)] = true
	}
	for _, path := range roots {
		if !existing[folderPathKey(path)] {
			s.state.Preferences.LibraryFolders = append(s.state.Preferences.LibraryFolders, path)
			existing[folderPathKey(path)] = true
		}
	}
	s.save()
	state := s.folderImport
	s.mu.Unlock()
	go s.importFolderWorker(ctx, roots)
	return state, nil
}
func (s *Studio) RescanLibraryFolders() (FolderImportState, error) {
	return s.ImportLibraryFolders(s.GetState().Preferences.LibraryFolders)
}
func (s *Studio) CancelFolderImport() {
	s.mu.Lock()
	if s.folderCancel != nil {
		s.folderCancel()
	}
	s.mu.Unlock()
}
func (s *Studio) RemoveLibraryFolder(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []string{}
	for _, value := range s.state.Preferences.LibraryFolders {
		if folderPathKey(value) != folderPathKey(path) {
			out = append(out, value)
		}
	}
	s.state.Preferences.LibraryFolders = out
	s.save()
	return nil
}
func (s *Studio) folderError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.folderImport.Failed++
	if len(s.folderImport.Errors) < 20 {
		s.folderImport.Errors = append(s.folderImport.Errors, message)
	}
}
func (s *Studio) collectFolderAudio(ctx context.Context, roots []string) []string {
	paths := []string{}
	seen := map[string]bool{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, item fs.DirEntry, walkErr error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if walkErr != nil {
				s.folderError("تعذر قراءة: " + path)
				return nil
			}
			if item.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if item.IsDir() {
				return nil
			}
			if !audioFileExtensions[strings.ToLower(filepath.Ext(path))] {
				return nil
			}
			info, err := item.Info()
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
			key := folderPathKey(path)
			if seen[key] {
				return nil
			}
			seen[key] = true
			paths = append(paths, path)
			s.mu.Lock()
			s.folderImport.Total = len(paths)
			s.folderImport.Current = path
			s.mu.Unlock()
			if len(paths) >= 200000 {
				return errors.New("حد الفحص 200000 ملف في المرة الواحدة")
			}
			return nil
		})
		if err != nil && ctx.Err() == nil {
			s.folderError(err.Error())
		}
		if ctx.Err() != nil {
			break
		}
	}
	return paths
}
func (s *Studio) importFolderWorker(ctx context.Context, roots []string) {
	defer func() {
		s.mu.Lock()
		s.folderImport.Running = false
		s.folderImport.Scanning = false
		s.folderImport.Cancelled = ctx.Err() != nil
		s.folderImport.Current = ""
		s.folderCancel = nil
		s.save()
		s.mu.Unlock()
	}()
	paths := s.collectFolderAudio(ctx, roots)
	s.mu.Lock()
	s.folderImport.Scanning = false
	s.mu.Unlock()
	for _, path := range paths {
		if ctx.Err() != nil {
			return
		}
		info, err := os.Stat(path)
		if err != nil {
			s.folderError("تعذر قراءة: " + path)
			continue
		}
		unchanged := false
		s.mu.Lock()
		for _, task := range s.state.Jobs {
			if task.Local && folderPathKey(task.Path) == folderPathKey(path) && task.Bytes == info.Size() && task.LocalModified == info.ModTime().UnixNano() {
				unchanged = true
				break
			}
		}
		s.folderImport.Current = path
		s.mu.Unlock()
		added := 0
		if !unchanged {
			added, err = s.importPaths(ctx, []string{path})
			if err != nil && ctx.Err() == nil {
				s.folderError("تعذر استيراد: " + filepath.Base(path))
			}
		}
		s.mu.Lock()
		s.folderImport.Processed++
		s.folderImport.Added += added
		if added == 0 && err == nil {
			s.folderImport.Skipped++
		}
		s.mu.Unlock()
	}
}
