# I Saw That

A simple file and directory watcher and backup utility written in Go.

## Features
- Watches a source directory recursively for changes
- Automatically creates timestamped backups of the source directory to a destination
- Debounces rapid file events to avoid redundant backups
- JSON metadata for backup history
- Extensible observer interface for notifications
- Comprehensive test suite

## Future Plans
- An optional GUI
- More options
- Watching multiple directories at once

### Build

Requires a modified version of fsnotify with recursion enabled that is not included in
this repository.

### Run

```
./i-saw-that source destination
```

## Project Structure

- `i-saw-that.go` — Command line interface
- `watcher.go` — Core watcher and backup logic
- `watcher_test.go` — Tests for watcher functionality
- `watcher_test_helpers.go` — Test helpers
- `notes/` — Miscellaneous notes and experiments

## Testing

```
go test -v
```


## Inspiration
Inspired by [AutoVer](https://www.beanland.net.au/AutoVer/) with the goal of being a
simple alternative that can be ran from the command line.
## License
MIT
