[![Coverage](http://gocover.io/_badge/github.com/flowonyx/bcnotify)](http://gocover.io/github.com/flowonyx/bcnotify) [![GoDoc](https://godoc.org/github.com/flowonyx/bcnotify?status.svg)](https://godoc.org/github.com/flowonyx/bcnotify)

# Layer on top of fsnotify
`bcnotify` is a layer on top of [fsnotify.v1](http://github.com/go-fsnotify/fsnotify) to make it easier to work with. Includes recursive adding of directories and filtering events.

## Is it production ready?
No. It has not yet been used in production and tests sometimes pass, sometimes fail for reasons I don't understand, although I believe the problem is with the test code, not the package code.

## How do I use it?
`bcnotify` monitors file system events. You begin by calling `NewFileSystemWatcher()` to get a `FileSystemWatcher`. You will want to make sure you call the `Close` method on that watcher to clean up when you are finished with it.

```go
fw, err := bcnotify.NewFileSystemWatcher()
// Error handling...
defer fw.Close()
```

To watch a specific file for events, use the `AddFile` method. You can specify the Op (operations) you want to monitor.

```go
err := fw.AddFile(filename, bcnotify.AllOps)
```
which is the same as
```go
err := fw.AddFile(filename, bcnotify.Create|bcnotify.Write|bcnotify.Chmod|bcnotify.Rename|bcnotify.Remove)
```

To monitor a directory for file events, use the `AddDir` method. You can add a directory recursively or not.

Call with:
* Path to the directory to monitor.
* File path filter to only monitor certain files in the directory. Use "" for no filtering.
* File operations to monitor.
* Whether to use recursion to add subdirectories.

```go
// Only monitor files that have the ".txt" extension,
// only the Create operation on files,
// and use recursion (add subdirectories).
err := fw.AddDir(dir, "*.txt", bcnotify.Create, true)
```

##### Filtering

File path filters use the `filepath.Match` method for matching. You can see the documentation for it [here](http://golang.org/pkg/path/filepath/#Match). Matching is performed only on the filename, the directory is not considered.

When you have added the files or directories you want to monitor, you then need to get the events. There are two methods for this.

#### WaitEvent

`WaitEvent` is a blocking method that waits until a filesystem event is fired. Unless you only want to receive one event, you will want to put it in a goroutine and within a loop.

```go
go func(){
  for {
    event, err := fw.WaitEvent()
    // Error handling...
    // Event handling
    fmt.Println(event.Name)
  }
}()
```

The `bcnotify.Event` that is returned is API compatible with `fsnotify.Event`.

#### NotifyEvent

`NotifyEvent` allows registering a function to receive all filesystem events.

```go
fw.NotifyEvent(func(event *bcnotify.Event, err error) {
  // Error handling...

  if event.Op&bcnotify.Create != 0 {
    // Handle create operation
  }
})
```

## Why the Name?
"BC" are the initials of my fiancé. I couldn't think of anything else to call it.
