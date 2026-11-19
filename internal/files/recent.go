package files

import (
	"path"
	"strconv"
	"strings"
	"time"
)

// RecentFilesLimit is how many files the recent list shows (ak pull, ak files t).
const RecentFilesLimit = 300

// RecentFilesQuery lists the newest files in MediaStore, like the Files app
// 'Recents' tab. It takes about a second even for a few rows: content starts
// a JVM on the device.
//
// A file just written (adb push, a shell, an app) has a row at once but no
// size or date_modified until MediaStore scans it, which can take a while: it
// is kept (size shown as "?") and sorted by date_added meanwhile, so the
// newest file is not missing or at the bottom. Empty files are still left out.
var RecentFilesQuery = "content query --uri content://media/external/file" +
	" --projection _data:date_modified:date_added:_size" +
	` --where "(format IS NULL OR format != 12289) AND (_size IS NULL OR _size > 0) AND _data NOT LIKE '%/Android/%' AND _data NOT LIKE '%/.%'"` +
	" --sort 'COALESCE(date_modified, date_added) DESC' 2>&1 | head -n " + strconv.Itoa(RecentFilesLimit)

// RecentFile is one row of RecentFilesQuery.
type RecentFile struct {
	Path     string
	Size     int64     // -1 if unknown
	Modified time.Time // zero if unknown
}

// ParseRecentFiles parses `content query` rows:
// "Row: 0 _data=<path>, date_modified=<ts>, date_added=<ts>, _size=<bytes>".
// NULL (not scanned yet) gives size -1 and the time from date_added.
func ParseRecentFiles(raw string) []RecentFile {
	var files []RecentFile
	for _, line := range strings.Split(raw, "\n") {
		if !strings.HasPrefix(line, "Row:") {
			continue
		}
		_, p, ok := strings.Cut(line, " _data=")
		if !ok {
			continue
		}
		f := RecentFile{Size: -1}
		if i := strings.Index(p, ", date_modified="); i >= 0 {
			rest := p[i+len(", date_modified="):]
			p = p[:i]
			ts, rest, _ := strings.Cut(rest, ",")
			if n, err := strconv.ParseInt(ts, 10, 64); err == nil {
				f.Modified = time.Unix(n, 0)
			} else if _, added, ok := strings.Cut(rest, " date_added="); ok {
				added, _, _ = strings.Cut(added, ",")
				if n, err := strconv.ParseInt(added, 10, 64); err == nil {
					f.Modified = time.Unix(n, 0)
				}
			}
		}
		if i := strings.LastIndex(line, ", _size="); i >= 0 {
			if n, err := strconv.ParseInt(strings.TrimSpace(line[i+len(", _size="):]), 10, 64); err == nil {
				f.Size = n
			}
		}
		if p == "" {
			continue
		}
		f.Path = p
		files = append(files, f)
	}
	return files
}

// sdcardPath shows the primary storage as /sdcard, the path ak files starts in.
func sdcardPath(p string) string {
	for _, prefix := range []string{"/storage/emulated/0", "/storage/self/primary"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return "/sdcard" + p[len(prefix):]
		}
	}
	return p
}

// mediaPath is p as MediaStore's _data stores it (/storage/emulated/0/...),
// the reverse of sdcardPath; false for paths outside shared storage.
func mediaPath(p string) (string, bool) {
	p = sdcardPath(p)
	if p == "/sdcard" || strings.HasPrefix(p, "/sdcard/") {
		return "/storage/emulated/0" + p[len("/sdcard"):], true
	}
	return "", false
}

// recentEntries turns query rows into entries with their folders shown.
func recentEntries(rows []RecentFile) []Entry {
	list := make([]Entry, 0, len(rows))
	for _, r := range rows {
		p := sdcardPath(r.Path)
		list = append(list, Entry{Name: path.Base(p), Path: p, Size: r.Size, ModTime: r.Modified, Folder: path.Dir(p)})
	}
	return list
}
