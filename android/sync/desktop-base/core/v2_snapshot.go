package main

import "errors"

func (s *Studio) GetTaskLog(id string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.state.Jobs {
		if t.ID == id {
			return append([]string{}, t.Log...), nil
		}
	}
	return nil, errors.New("المسار غير موجود")
}

// Copy mutable slices/maps directly. A JSON round trip for every UI poll costs
// significantly more allocation for large libraries and repeats RPC encoding.
func cloneSnapshot(source Snapshot) Snapshot {
	r := source
	r.Jobs = make([]Task, len(source.Jobs))
	copy(r.Jobs, source.Jobs)
	for i := range r.Jobs {
		r.Jobs[i].Song.Genres = append([]string(nil), source.Jobs[i].Song.Genres...)
		logs := source.Jobs[i].Log
		if len(logs) > 50 {
			logs = logs[len(logs)-50:]
		}
		r.Jobs[i].Log = append([]string(nil), logs...)
	}
	r.Preferences.LyricProviders = append([]string(nil), source.Preferences.LyricProviders...)
	r.Preferences.LibraryFolders = append([]string(nil), source.Preferences.LibraryFolders...)
	r.AlignmentQueue = append([]string(nil), source.AlignmentQueue...)
	r.TranslationQueue = append([]TranslationRequest(nil), source.TranslationQueue...)
	r.Cooldowns = map[string]int64{}
	for k, v := range source.Cooldowns {
		r.Cooldowns[k] = v
	}
	return r
}
