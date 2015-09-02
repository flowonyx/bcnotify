package bcnotify

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Utility function for making a test directory
func makeTestDir(t *testing.T) string {
	dir, err := ioutil.TempDir(".", "test")
	if err != nil {
		t.Error(err)
	}
	return dir
}

func TestMain(m *testing.M) {
	code := m.Run()
	// Give time to clean up any leftover test* directories
	time.Sleep(500 * time.Millisecond)
	os.Exit(code)
}

// Make sure the Event.String() method returns the proper string
func TestEventString(t *testing.T) {
	event := Event{}
	event.Name = "testfile.txt"
	event.Op = Write
	expected := `"testfile.txt": WRITE`
	if event.String() != expected {
		t.Fatalf("Wanted %q got %q", expected, event.String())
	}
}

// Make sure private method findWatchPath works as advertized
func TestFindWatchPath(t *testing.T) {
	fw, _ := NewFileSystemWatcher()
	defer fw.Close()
	wp := []string{"test.txt", "testdir", "testdir/test.txt"}
	for _, test := range wp {
		fw.watchPaths = append(fw.watchPaths, watchPath{path: test})
	}
	p := fw.findWatchPath("none")
	if p != nil {
		t.Fatal("Should not have found a watchPath")
	}
	for _, test := range wp {
		p = fw.findWatchPath(test)
		if p.path != test {
			t.Fatalf("wanted %s got %s", test, p.path)
		}

	}
}

// Make sure private method filterByPattern works as advertized
func TestFilterByPattern(t *testing.T) {
	fw, _ := NewFileSystemWatcher()
	defer fw.Close()
	wp := []string{"test.txt", "testdir", "testdir/test.txt"}
	for _, test := range wp {
		fw.watchPaths = append(fw.watchPaths, watchPath{path: test, pattern: "*test*"})
	}
	if fw.filterByPattern("none") {
		t.Fatal("filterByPattern returned true when it should have returned false")
	}
	for _, p := range wp {
		if !fw.filterByPattern(p) {
			t.Fatal("filterByPattern did not return true for", p)
		}
	}

}

// Make sure private method filterByOp works as advertized
func TestFilterByOp(t *testing.T) {
	fw, _ := NewFileSystemWatcher()
	defer fw.Close()
	wp := []string{"test.txt", "testdir", "testdir/test.txt"}
	for _, test := range wp {
		fw.watchPaths = append(fw.watchPaths, watchPath{path: test, ops: Write})
	}
	if fw.filterByOp("none", Write) {
		t.Fatal("filterByOp returned true when it should have returned false")
	}
	for _, p := range wp {
		if fw.filterByOp(p, Create) {
			t.Fatal("filterByOp returned true for", p)
		}
	}
	for _, p := range wp {
		if !fw.filterByOp(p, Write) {
			t.Fatal("filterByOp did not return true for", p)
		}
	}

}

// Make sure the initializer works properly.
func TestNewFileSystemWatcher(t *testing.T) {
	fw, _ := NewFileSystemWatcher()
	defer fw.Close()
	if fw.close == nil {
		t.Fatal("NewFileSystemWatcher did not initialize close")
	}
	if fw.watcher == nil {
		t.Fatal("NewFileSystemWatcher did not initialize watcher")
	}
}

// Make sure that adding a non-existing file will return an error
func TestFileSystemWatcherAddFileNonExisting(t *testing.T) {
	fw, _ := NewFileSystemWatcher()
	defer fw.Close()
	filename := "testfile.txt"
	err := fw.AddFile(filename, AllOps)
	if err == nil {
		t.Fatal("AddFile should not allow adding of non-existing files")
	}
}

// Make sure that adding a directory with AddFile will return an error
func TestFileSystemWatcherAddFileWithDirectory(t *testing.T) {
	fw, _ := NewFileSystemWatcher()
	defer fw.Close()
	dir := "./"
	err := fw.AddFile(dir, AllOps)
	if err == nil {
		t.Fatal("AddFile should not allow adding of directories")
	}
}

// Make sure that adding a file with AddFile works correctly
func TestFileSystemWatcherAddFile(t *testing.T) {
	// Setup the test directory
	dir := makeTestDir(t)
	defer os.RemoveAll(dir)

	fw, _ := NewFileSystemWatcher()
	defer fw.Close()

	filename := filepath.Join(dir, "test.txt")

	// Actually write the file
	err := ioutil.WriteFile(filename, []byte("test"), 0700)
	if err != nil {
		t.Error(err)
	}
	err = fw.AddFile(filename, AllOps)
	if err != nil {
		t.Error(err)
	}

	done := make(chan struct{})
	fw.NotifyEvent(func(event *Event, err error) {

		// Make sure we send the done channel a signal at the end.
		defer func() {
			done <- struct{}{}
		}()

		if err != nil {
			t.Error(err)
		}
		if event == nil {
			t.Fatal("event should not be nil")
		}
		if event.Name != filename {
			t.Fatalf("event does not have correct filename. Wanted %s got %s", filename, event.Name)
		}
	})

	// Write the file again
	ioutil.WriteFile(filename, []byte("test"), 0700)

	// Wait until the event is caught and tested or we time out.
	select {
	case <-done:
		return
	case <-time.Tick(100 * time.Millisecond):
		t.Fatal("Timed out")
	}

}

// Make sure that adding file with AddFile works correctly with filtering by Op
func TestFileSystemWatcherAddFileOpFilter(t *testing.T) {
	// Setup the test directory
	dir := makeTestDir(t)
	defer os.RemoveAll(dir)

	fw, _ := NewFileSystemWatcher()
	defer fw.Close()

	filename := filepath.Join(dir, "test.txt")

	// Actually write the file
	err := ioutil.WriteFile(filename, []byte("test"), 0700)
	if err != nil {
		t.Error(err)
	}
	err = fw.AddFile(filename, Chmod)
	if err != nil {
		t.Error(err)
	}

	done := make(chan struct{})
	fw.NotifyEvent(func(event *Event, err error) {
		// Make sure we send the done channel a signal at the end.
		defer func() {
			done <- struct{}{}
		}()

		if err != nil {
			t.Error(err)
		}
		if event == nil {
			t.Fatal("event should not be nil")
		}
		if event.Name != filename {
			t.Fatalf("event does not have correct filename. Wanted %s got %s", filename, event.Name)
		}
		if event.Op&Chmod != Chmod {
			t.Fatal("Got wrong event:", event.Op)
		}
	})

	doFileOps(t, filename)

	// Wait until the event is caught and tested or we time out.
	select {
	case <-done:
		return
	case <-time.Tick(100 * time.Millisecond):
		t.Fatal("Timed out")
	}

}

// Make sure that removing a file with RemoveFile works correctly
func TestFileSystemWatcherRemoveFile(t *testing.T) {
	// Setup the test directory
	dir := makeTestDir(t)
	defer os.RemoveAll(dir)

	fw, _ := NewFileSystemWatcher()
	defer fw.Close()

	filename := filepath.Join(dir, "test.txt")

	// Actually write the file
	err := ioutil.WriteFile(filename, []byte("test"), 0700)
	if err != nil {
		t.Error(err)
	}
	err = fw.AddFile(filename, AllOps)
	if err != nil {
		t.Error(err)
	}

	err = fw.RemoveFile(filename)
	if err != nil {
		t.Error(err)
	}

	fw.NotifyEvent(func(event *Event, err error) {
		t.Fatal("Should not have notified of event.")
	})

	// Write the file again
	ioutil.WriteFile(filename, []byte("test"), 0700)

	// Wait until the event is caught and tested or we time out.
	select {
	case <-time.Tick(100 * time.Millisecond):
		t.Log("timed out, which we wanted")
	}

}

// Make sure adding directories recursively works
func TestFileSystemWatcherAddDirRecursive(t *testing.T) {
	// Setup the test directory
	dir := makeTestDir(t)
	defer os.RemoveAll(dir)

	fw, _ := NewFileSystemWatcher()
	defer fw.Close()

	// Create a subdirectory for testing recursive adds
	os.MkdirAll(filepath.Join(dir, "sub"), 0700)
	// Filter on .txt files and do it recursively
	err := fw.AddDir(dir, "*.txt", AllOps, true)
	if err != nil {
		t.Error(err)
	}

	// Setup the NotifyEvent function
	filename := filepath.Join(dir, "sub", "testfile.txt")
	done := make(chan struct{})
	isdone := false
	fw.NotifyEvent(func(event *Event, err error) {
		if isdone {
			return
		}
		// Make sure we send the done channel a signal at the end.
		defer func() {
			isdone = true
			done <- struct{}{}
		}()

		if err != nil {
			t.Error(err)
		}
		if event == nil {
			t.Fatal("event should not be nil")
		}
		if event.Name != filename {
			t.Fatalf("event does not have correct filename. Wanted %s got %s", filename, event.Name)
		}
	})

	// Actually write the file
	ioutil.WriteFile(filename, []byte("test"), 0700)

	// Wait until the event is caught and tested or we time out.
	select {
	case <-done:
		return
	case <-time.Tick(100 * time.Millisecond):
		t.Fatal("Timed out")
	}
}

// Make sure removing directories with recursion works
func TestFileSystemWatcherRemoveDirRecursive(t *testing.T) {
	// Setup the test directory
	dir := makeTestDir(t)
	defer os.RemoveAll(dir)

	fw, _ := NewFileSystemWatcher()
	defer fw.Close()

	err := os.MkdirAll(filepath.Join(dir, "sub"), 0700)
	if err != nil {
		t.Error(err)
	}
	// Filter on .txt files and do it recursively
	err = fw.AddDir(dir, "*.txt", AllOps, true)
	if err != nil {
		t.Error(err)
	}

	err = fw.RemoveDir(dir, true)
	if err != nil {
		t.Error(err)
	}

	// Setup the NotifyEvent function
	fw.NotifyEvent(func(event *Event, err error) {
		t.Fatal("event still fired")
	})

	filename := filepath.Join(dir, "sub", "testfile.txt")
	// Actually write the file
	ioutil.WriteFile(filename, []byte("test"), 0700)

	filename = filepath.Join(dir, "testfile.txt")
	ioutil.WriteFile(filename, []byte("test"), 0700)

	// Wait until the event is caught and tested or we time out.
	select {
	case <-time.Tick(100 * time.Millisecond):
		if !t.Failed() {
			t.Log("Timed out, which is what we wanted")
		}
	}
}

// Make sure adding directories without recursion works
func TestFileSystemWatcherAddDirNotRecursive(t *testing.T) {
	// Setup the test directory
	dir := makeTestDir(t)
	defer os.RemoveAll(dir)

	fw, _ := NewFileSystemWatcher()
	defer fw.Close()

	// Create a subdirectory for testing recursive adds
	os.MkdirAll(filepath.Join(dir, "sub"), 0700)
	// Do NOT do it recursively
	err := fw.AddDir(dir, "", AllOps, false)
	if err != nil {
		t.Error(err)
	}

	// Setup the NotifyEvent function
	filename := filepath.Join(dir, "sub", "testfile.txt")
	done := make(chan struct{})
	isdone := false
	fw.NotifyEvent(func(event *Event, err error) {
		if isdone {
			return
		}
		isdone = true
		done <- struct{}{}
	})

	// Actually write the file
	ioutil.WriteFile(filename, []byte("test"), 0700)

	// Wait until the event is caught and tested or we time out.
	select {
	case <-done:
		t.Fatal("Should not have received notification for subdirectory")
	case <-time.Tick(100 * time.Millisecond):
		return
	}
}

// Make sure removing directories without recursion works
func TestFileSystemWatcherRemoveDirNotRecursive(t *testing.T) {
	// Setup the test directory
	dir := makeTestDir(t)
	defer os.RemoveAll(dir)

	fw, _ := NewFileSystemWatcher()
	defer fw.Close()

	// Create a subdirectory for testing non-recursive removes
	os.MkdirAll(filepath.Join(dir, "sub"), 0700)
	// Filter on .txt files and do it non-recursively
	err := fw.AddDir(dir, "*.txt", AllOps, true)
	if err != nil {
		t.Error(err)
	}

	err = fw.RemoveDir(dir, false)
	if err != nil {
		t.Error(err)
	}

	done := make(chan struct{})
	// Setup the NotifyEvent function
	fw.NotifyEvent(func(event *Event, err error) {
		filename := filepath.Join(dir, "testfile.txt")
		if event.Name == filename {
			t.Fatal("event still fired")
		}
		done <- struct{}{}
	})

	filename := filepath.Join(dir, "testfile.txt")
	ioutil.WriteFile(filename, []byte("test"), 0700)

	filename = filepath.Join(dir, "sub", "testfile.txt")
	ioutil.WriteFile(filename, []byte("test"), 0700)

	// Wait until the event is caught and tested or we time out.
	select {
	case <-done:
		return
	case <-time.Tick(100 * time.Millisecond):
		t.Fatal("Timed out")
	}
}

// Make sure WaitEvent works
func TestFileSystemWatcherWaitEvent(t *testing.T) {
	// Setup the test directory
	dir := makeTestDir(t)
	defer os.RemoveAll(dir)

	fw, _ := NewFileSystemWatcher()
	defer fw.Close()

	// Add directory without any filtering, without recursion
	err := fw.AddDir(dir, "", AllOps, false)
	if err != nil {
		t.Error(err)
	}

	// Setup goroutine to wait for the event
	done := make(chan struct{})
	go func() {
		// Make sure we send the done channel a signal at the end.
		defer func() {
			done <- struct{}{}
		}()

		event, err := fw.WaitEvent()
		if err != nil {
			t.Error(err)
		}

		if event == nil {
			t.Fatal("WaitEvent returned without error but with nil event")
		}

	}()

	// Actually write the file
	ioutil.WriteFile(filepath.Join(dir, "testfile"), []byte("test"), 0700)

	// Wait until the event is received or we timeout
	select {
	case <-done:
		return
	case <-time.Tick(100 * time.Millisecond):
		t.Fatal("Timed out")
	}
}

// Make sure NotifyEvent works
func TestFileSystemWatcherNotifyEvent(t *testing.T) {
	// Setup the test directory
	dir := makeTestDir(t)
	defer os.RemoveAll(dir)

	fw, _ := NewFileSystemWatcher()
	defer fw.Close()

	// Add the directory to the watcher
	err := fw.AddDir(dir, "", AllOps, false)
	if err != nil {
		t.Error(err)
	}

	// Setup the NotifyEvent function
	done := make(chan struct{})
	isdone := false
	fw.NotifyEvent(func(event *Event, err error) {
		if isdone {
			return
		}
		// Make sure we send the done channel a signal at the end.
		defer func() {
			isdone = true
			done <- struct{}{}
		}()

		if err != nil {
			t.Error(err)
		}

		if event == nil {
			t.Fatal("event should not be nil")
		}

	})

	// Actually write the file
	ioutil.WriteFile(filepath.Join(dir, "testfile"), []byte("test"), 0700)

	// Wait for the event to be received or we timeout
	select {
	case <-done:
		return
	case <-time.Tick(100 * time.Millisecond):
		t.Fatal("Timed out")
	}
}

// Make sure the watcher works with multiple events
func TestFileSystemWatcherMultipleCreates(t *testing.T) {
	// Setup test directory
	dir := makeTestDir(t)
	defer os.RemoveAll(dir)

	fw, _ := NewFileSystemWatcher()
	defer fw.Close()

	// Add directory to file watcher, filtering on Create so that we only get one
	// event for each file.
	err := fw.AddDir(dir, "", Create, false)
	if err != nil {
		t.Error(err)
	}

	// We use a WaitGroup to tell when we have received all the events for the
	// created files.
	var wait sync.WaitGroup

	// counter checks how many times the NotifyEvent function is called.
	// This may be redundant wit the WaitGroup.
	var counter int64
	fw.NotifyEvent(func(event *Event, err error) {
		// Make sure we set Done at the end.
		defer func() {
			wait.Done()
			atomic.AddInt64(&counter, 1)
		}()

		if err != nil {
			t.Error(err)
		}

		if event == nil {
			t.Fatal("event should not be nil")
		}

	})

	// maxCount is the number of files to write to disk and the number of events
	// we want to receive.
	maxCount := 100
	wait.Add(maxCount)
	for i := 0; i < maxCount; i++ {
		go func(i int) {
			// Write the files to disk
			filename := fmt.Sprintf("%s%d.txt", "test", i)
			filename = filepath.Join(dir, filename)
			ioutil.WriteFile(filename, []byte("test"), 0700)
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wait.Wait()
		c := atomic.LoadInt64(&counter)
		if c != int64(maxCount) {
			t.Fatalf("Wanted %d events but got %d", maxCount, c)
		}
		done <- struct{}{}
	}()

	select {
	case <-done:
		return
	case <-time.Tick(100 * time.Millisecond):
		t.Fatal("Timed out")
	}
}

// Perform all file operations (taken from fsnotify tests)
func doFileOps(t *testing.T, path string) {
	fi, err := os.Stat(path)
	if err != nil {
		t.Error(err)
	}

	filename := path
	if fi.IsDir() {
		filename = filepath.Join(path, "testfile")
	}
	// Should fire Create and Write Ops
	var f *os.File
	f, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatal(err)
	}
	f.Sync()

	time.Sleep(5 * time.Millisecond)
	f.WriteString("test")
	f.Sync()
	f.Close()

	time.Sleep(5 * time.Millisecond)
	// Should fire Chmod
	os.Chmod(filename, 0666)

	time.Sleep(5 * time.Millisecond)

	// Should fire Rename
	os.Rename(filename, filename+".new")

	time.Sleep(5 * time.Millisecond)

	// Should fire Remove
	os.Remove(filename + ".new")

}

// Make sure the AllOps Op filter works
func TestOpFilterAllOps(t *testing.T) {

	dir := makeTestDir(t)
	defer os.RemoveAll(dir)

	fw, _ := NewFileSystemWatcher()
	defer fw.Close()

	err := fw.AddDir(dir, "", AllOps, false)
	if err != nil {
		t.Error(err)
	}
	var wait sync.WaitGroup
	wait.Add(5) // One for each Op

	var mu sync.Mutex
	ops := map[Op]struct{}{
		Create: struct{}{},
		Write:  struct{}{},
		Remove: struct{}{},
		Rename: struct{}{},
		Chmod:  struct{}{},
	}

	fw.NotifyEvent(func(event *Event, err error) {
		mu.Lock()
		defer mu.Unlock()

		if event == nil {
			t.Fatal("event should not be nil")
		}

		if event.Op&Create == Create {
			if _, ok := ops[Create]; !ok {
				return
			}
			delete(ops, Create)
		}
		if event.Op&Write == Write {
			if _, ok := ops[Write]; !ok {
				return
			}
			delete(ops, Write)
		}
		if event.Op&Chmod == Chmod {
			if _, ok := ops[Chmod]; !ok {
				return
			}
			delete(ops, Chmod)
		}
		if event.Op&Rename == Rename {
			if _, ok := ops[Rename]; !ok {
				return
			}
			delete(ops, Rename)
		}
		if event.Op&Remove == Remove {
			if _, ok := ops[Remove]; !ok {
				return
			}
			delete(ops, Remove)
		}

		wait.Done()
	})

	done := make(chan struct{})
	go func() {
		wait.Wait()
		done <- struct{}{}
	}()

	doFileOps(t, dir)

	select {
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		if len(ops) > 0 {
			t.Fatal("Did not catch all Ops:", ops)
		}
		return
	case <-time.Tick(5 * time.Second):
		mu.Lock()
		defer mu.Unlock()
		t.Fatal("Timed out with Ops:", ops)
	}
}

// Make sure the op filter works for the Op passed in
func testOpFilter(t *testing.T, op Op) {

	dir := makeTestDir(t)
	defer os.RemoveAll(dir)

	fw, _ := NewFileSystemWatcher()
	defer fw.Close()

	err := fw.AddDir(dir, "", op, false)
	if err != nil {
		t.Error(err)
	}

	fw.NotifyEvent(func(event *Event, err error) {

		if event == nil {
			t.Fatal("event should not be nil")
		}

		if err != nil {
			t.Error(err)
		}

		if event.Op != op {
			t.Fatal("Notified of wrong event:", event.String())
		}

	})

	doFileOps(t, dir)

	time.Sleep(100 * time.Millisecond)

}

// Test all the Op filters (except AllOps)
func TestOpFilters(t *testing.T) {
	ops := []Op{Create, Write, Rename, Remove, Chmod}
	for i := range ops {
		go func(i int) {
			testOpFilter(t, ops[i])
		}(i)
	}

}

// Make sure file pattern filtering works by doing operations on a matching file // and a non-matching file.
func testPatternFilter(t *testing.T, pattern, fileMatch, fileNoMatch string) {
	dir := makeTestDir(t)
	defer os.RemoveAll(dir)

	fileMatch = filepath.Join(dir, fileMatch)
	fileNoMatch = filepath.Join(dir, fileNoMatch)

	fw, _ := NewFileSystemWatcher()
	defer fw.Close()

	err := fw.AddDir(dir, pattern, AllOps, false)
	if err != nil {
		t.Error(err)
	}

	fw.NotifyEvent(func(event *Event, err error) {

		if event == nil {
			t.Fatal("event should not be nil")
		}

		if err != nil {
			t.Error(err)
		}

		if event.Name != fileMatch {
			t.Fatal("Notified of wrong file:", event.String())
		}

	})

	err = ioutil.WriteFile(fileNoMatch, []byte("test"), 0700)
	if err != nil {
		t.Error(err)
	}

	time.Sleep(5 * time.Millisecond)

	err = ioutil.WriteFile(fileMatch, []byte("test"), 0700)
	if err != nil {
		t.Error(err)
	}

	time.Sleep(100 * time.Millisecond)
}

// Make sure file pattern filtering works
func TestPatternFilters(t *testing.T) {
	tests := []struct {
		pattern     string
		matching    string
		nonmatching string
	}{
		{
			pattern:     "*.txt",
			matching:    "test.txt",
			nonmatching: "test.ini",
		},
		{
			pattern:     "test.txt",
			matching:    "test.txt",
			nonmatching: "test2.txt",
		},
		{
			pattern:     "test*.txt",
			matching:    "test2.txt",
			nonmatching: "tes.ini",
		},
		{
			pattern:     "*thing.txt",
			matching:    "somethin.txt",
			nonmatching: "thingsome.txt",
		},
	}
	for i := range tests {
		go func(i int) {
			test := tests[i]
			testPatternFilter(t, test.pattern, test.matching, test.nonmatching)
		}(i)
	}
}
