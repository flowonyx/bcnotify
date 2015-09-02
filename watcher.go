package bcnotify

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/facebookgo/stackerr"

	"gopkg.in/fsnotify.v1"
)

// ErrWatcherClosed is returned to allow for clean shutting down of a watcher.
var ErrWatcherClosed = fmt.Errorf("FileSystemWatcher closed")

// watchPath represents a single path Added to the watcher
type watchPath struct {
	path    string // Path to watch
	pattern string // Filename pattern to filter on (blank if no filter)
	ops     Op     // Operation on which to filter (AllOps if no filter)
	isdir   bool   // True if this is a directory
}

// FileSystemWatcher represents a structure used to watch files on the file system.
type FileSystemWatcher struct {
	watcher    *fsnotify.Watcher // internal watcher that does all the real work
	watchPaths []watchPath       // paths that are watched

	closedMu sync.Mutex
	isclosed bool
	close    chan struct{}
}

// Event represents a single file system notification.
type Event struct {
	event fsnotify.Event
	Name  string // Relative path to the file or directory.
	Op    Op     // File operation that triggered the event.
}

func (e Event) String() string {
	e.event.Name = e.Name
	e.event.Op = fsnotify.Op(e.Op)
	return e.event.String()
}

// Op describes a set of file operations.
type Op uint32

// These are the generalized file operations that can trigger a notification.
const (
	Create Op = 1 << iota
	Write
	Remove
	Rename
	Chmod

	AllOps = Create | Write | Remove | Rename | Chmod
)

// wrapEvent takes an fsnotify.Event and returns a bcnotify.Event
func wrapEvent(e fsnotify.Event) *Event {
	return &Event{event: e, Name: e.Name, Op: Op(e.Op)}
}

// findWatchPath searches the FileSystemWatcher's watchPaths slice for one
// that fits the given path and returns that watchPath.
func (fw *FileSystemWatcher) findWatchPath(path string) *watchPath {
	// Check for full path first (if watching the specific file, this needs to go
	// before the directory)
	for _, p := range fw.watchPaths {
		if filepath.Clean(path) == filepath.Clean(p.path) {
			return &p
		}
	}
	// Now check the directories
	for _, p := range fw.watchPaths {
		d := filepath.Dir(path)
		if filepath.Clean(d) == filepath.Clean(p.path) {
			return &p
		}
	}
	return nil
}

// filterByPattern takes a path and determines if it fits the filter given for
// that path.
func (fw *FileSystemWatcher) filterByPattern(path string) bool {
	p := fw.findWatchPath(path)
	if p == nil {
		return false
	}

	// If there was no filter pattern given, we allow it.
	if len(p.pattern) == 0 {
		return true
	}

	// If this is a file that has been specifically added, we do not try any
	// filters and just allow it.
	if !p.isdir {
		return true
	}

	// Run the filter on the filename only.
	_, path = filepath.Split(path)
	match, err := filepath.Match(p.pattern, path)
	if err != nil {
		fmt.Println(err)
		return false
	}
	if match {
		return true
	}
	return false
}

// filterByOp simply tests whether the given operation is included in the ones
// set in the watchPath.
func (fw *FileSystemWatcher) filterByOp(path string, op Op) bool {
	p := fw.findWatchPath(path)
	if p == nil {
		return false
	}
	// This tests whether the given Op is included in the Op list
	// (e.g. match Create against Create|Write)
	if p.ops&op == op {
		return true
	}
	return false
}

// NewFileSystemWatcher returns an initialized *FileSystemWatcher.
func NewFileSystemWatcher() (*FileSystemWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, stackerr.Wrap(err)
	}
	return &FileSystemWatcher{watcher: w, close: make(chan struct{})}, nil
}

// Close closes the system resources for this FileSystemWatcher
func (fw *FileSystemWatcher) Close() error {
	fw.closedMu.Lock()
	defer fw.closedMu.Unlock()
	if fw.isclosed {
		return nil
	}
	fw.isclosed = true
	close(fw.close)
	return fw.watcher.Close()
}

// WaitEvent blocks and waits until an event or error comes through.
// This needs to be called in a go routine, probably in a loop.
func (fw *FileSystemWatcher) WaitEvent() (*Event, error) {
	for {
		select {
		case event := <-fw.watcher.Events:
			if fw.filterByOp(event.Name, Op(event.Op)) {
				if fw.filterByPattern(event.Name) {
					return wrapEvent(event), nil
				}
			}
			continue
		case err := <-fw.watcher.Errors:
			return nil, stackerr.Wrap(err)
		case <-fw.close:
			return nil, ErrWatcherClosed
		}
	}
}

// NotifyEvent accepts a function that takes a *bcnotify.Event and error
// and calls that function whenever an event or error happens.
func (fw *FileSystemWatcher) NotifyEvent(notify func(*Event, error)) {
	go func() {
		for {
			event, err := fw.WaitEvent()
			if err != nil {
				// ErrWatcherClosed is returned when the FileSystemWatcher is closed, so // we just want to return out of this loop and function in that case.
				if err == ErrWatcherClosed {
					return
				}
				notify(nil, err)
				continue
			}
			notify(event, nil)
		}
	}()
}

// isDir returns whether a given path is a directory and an error if one occurs.
func isDir(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, stackerr.Wrap(err)
	}
	return fi.IsDir(), nil
}

// AddFile adds a file to be watched along with an Op on which to filter events, // returning an error if any.
func (fw *FileSystemWatcher) AddFile(path string, ops Op) error {
	// Check if this is a directory and return an error if it is.
	if isdir, err := isDir(path); err == nil && isdir {
		return fmt.Errorf("Use AddDir instead for %s", path)
	} else if err != nil {
		return stackerr.Wrap(err)
	}
	// Add the path to the internal fsnotify watcher.
	err := fw.watcher.Add(path)
	if err != nil {
		return stackerr.Wrap(err)
	}
	// Add the path to watchPaths so we can search for it later and see
	// its configuration.
	fw.watchPaths = append(fw.watchPaths, watchPath{path: path, ops: ops})
	return nil
}

// RemoveFile removes a file from being watched and returns and error if any.
func (fw *FileSystemWatcher) RemoveFile(path string) error {
	// Check if this is a directory and return an error if it is.
	if isdir, err := isDir(path); err == nil && isdir {
		return fmt.Errorf("Use RemoveDir instead for %s", path)
	} else if err != nil {
		return stackerr.Wrap(err)
	}
	// Remove the path from the internal fsnotify watcher.
	err := fw.watcher.Remove(path)
	if err != nil {
		return stackerr.Wrap(err)
	}
	fw.watchPaths = removePath(fw.watchPaths, path)
	return nil
}

func removePath(paths []watchPath, path string) []watchPath {
	// Remove the path from watchPaths
	index := 0
	found := false
	for index = 0; index < len(paths); index++ {
		if paths[index].path == path {
			found = true
			break
		}
	}
	if found {
		paths = append(paths[0:index], paths[index+1:]...)
	}
	return paths
}

// addDir adds a directory path to watch with a filename pattern on which to
// filter and an Op on which to filter events.
func (fw *FileSystemWatcher) addDir(path, pattern string, ops Op) error {
	// First ensure that the given path really is a directory.
	if isdir, err := isDir(path); err == nil && !isdir {
		return fmt.Errorf("Use AddFile instead for %s", path)
	} else if err != nil {
		return stackerr.Wrap(err)
	}
	// Add path to internal fsnotify watcher.
	err := fw.watcher.Add(path)
	if err != nil {
		return stackerr.Wrap(err)
	}

	// Add to watchPaths so we can find it later with its configuration.
	fw.watchPaths = append(fw.watchPaths, watchPath{path: path, pattern: pattern, ops: ops, isdir: true})

	return nil
}

// AddDir adds a directory to be watched, returning an error if any.
// It allows a filter to be specified on which files to watch.
// It also allows recursive watching.
func (fw *FileSystemWatcher) AddDir(path, pattern string, ops Op, recursive bool) error {

	// Add the given path to be watched. addDir will perform checking for us to
	// ensure that the path really is a directory.
	err := fw.addDir(path, pattern, ops)
	if err != nil {
		return stackerr.Wrap(err)
	}

	if recursive {
		err = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if info.IsDir() {
				// Subdirectories inherit the filename pattern and ops from the parent.
				if e := fw.addDir(p, pattern, ops); err != nil {
					return stackerr.Wrap(e)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// RemoveDir removes a directory from the watcher and returns error if any
func (fw *FileSystemWatcher) removeDir(path string) error {
	// First ensure that the given path really is a directory.
	if isdir, err := isDir(path); err == nil && !isdir {
		return fmt.Errorf("Use RemoveFile instead for %s", path)
	} else if err != nil {
		return stackerr.Wrap(err)
	}
	// Remove path from internal fsnotify watcher.
	err := fw.watcher.Remove(path)
	if err != nil {
		return stackerr.Wrap(err)
	}

	// Add to watchPaths so we can find it later with its configuration.
	fw.watchPaths = removePath(fw.watchPaths, path)

	return nil
}

// RemoveDir removes a directory from being watched, returning an error if any.
// It also allows recursive removal.
func (fw *FileSystemWatcher) RemoveDir(path string, recursive bool) error {

	// Remove the given path from being watched.
	err := fw.removeDir(path)
	if err != nil {
		return stackerr.Wrap(err)
	}

	if recursive {
		err = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if info.IsDir() {
				if e := fw.removeDir(p); err != nil {
					return stackerr.Wrap(e)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}
