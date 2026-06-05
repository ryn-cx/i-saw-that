package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrorInvalidNameV2 = fmt.Errorf("error validating name")
var ErrorInvalidWaitTime = fmt.Errorf("error validating wait time")
var ErrorInvalidSource = fmt.Errorf("error validating source")
var ErrorInvalidDestination = fmt.Errorf("error validating destination")
var ErrorInvalidFolderFormat = fmt.Errorf("error validating folder format")

func validateName(name string, errs *error) {
	if name == "" {
		*errs = errors.Join(*errs, fmt.Errorf("%w: name cannot be empty", ErrorInvalidNameV2))
	}
}

func validateWaitTime(waitTime float64, errs *error) {
	if waitTime <= 0 {
		*errs = errors.Join(*errs, fmt.Errorf("%w: wait time must be at least 0 seconds", ErrorInvalidWaitTime))
	}
}

// Validate the folder format.
// Make sure that file names cannot overlap.
// Make sure the format is supported by the filesystem.
func validateFolderFormat(waitTime float64, folderFormat string, errs *error) {
	// Attempt to create two different times exactly one waitTime apart and make sure
	// that the names are different to avoid potential collisions
	seconds := int64(waitTime)
	nanoseconds := int64((waitTime - float64(seconds)) * 1e9)
	format1 := time.Unix(0, 0).Format(folderFormat)
	format2 := time.Unix(seconds, nanoseconds).Format(folderFormat)
	if format1 == format2 {
		err := fmt.Errorf("%w: folder format lacks adequate precision for wait time", ErrorInvalidFolderFormat)
		*errs = errors.Join(*errs, err)
	}

	validateDir(folderFormat, ErrorInvalidFolderFormat, errs)
}

// Validate a path is a directory.
// The path must be supported by the filesystem.
// The path must not be a file.
// If the path does not exist, it will be created.
func validateDir(path string, invalidNameError error, errs *error) {
	var pathErr *os.PathError

	info, err := os.Stat(path)

	// errors.As(err, &pathErr) returns true if the file does not exist, so it must be
	// checked after checking if the file exists
	// os.IsNotExist(err) returns false if the name is invalid so it can be checked
	// before checking if the name is invalid
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			*errs = errors.Join(*errs, err)
		}
	} else if errors.As(err, &pathErr) {
		*errs = errors.Join(*errs, fmt.Errorf("%w: invalid name: %w", invalidNameError, err))
	} else if err == nil && !info.IsDir() {
		*errs = errors.Join(*errs, fmt.Errorf("%w: %s exists but is not a directory", invalidNameError, path))
	} else if err != nil {
		*errs = errors.Join(*errs, fmt.Errorf("%w: %w", invalidNameError, err))
	}
}

// TODO: Deprecate
func validateDirOld(path string, invalidNameError error) error {
	var errs error
	var pathErr *os.PathError

	info, err := os.Stat(path)

	// errors.As(err, &pathErr) returns true if the file does not exist, so it must be
	// checked after checking if the file exists
	// os.IsNotExist(err) returns false if the name is invalid so it can be checked
	// before checking if the name is invalid
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			errs = errors.Join(errs, err)
		}
	} else if errors.As(err, &pathErr) {
		errs = errors.Join(errs, fmt.Errorf("%w: invalid name: %w", invalidNameError, err))
	} else if err == nil && !info.IsDir() {
		errs = errors.Join(errs, fmt.Errorf("%w: %s exists but is not a directory", invalidNameError, path))
	} else if err != nil {
		errs = errors.Join(errs, fmt.Errorf("%w: %w", invalidNameError, err))
	}

	return errs
}

// Validate source and destination directories.
// The values rely on one another so both must be validated at the same time.
// The paths must be supported by the filesystem.
// The paths must not be a file.
// If the paths do not exist, they will be created.
// The paths must not be the same.
// The destination must not be inside the source.
func validateSourceAndDestination(source string, destination string, errs *error) {
	// Generic directory validation
	*errs = errors.Join(*errs, validateDirOld(source, ErrorInvalidSource))
	*errs = errors.Join(*errs, validateDirOld(destination, ErrorInvalidDestination))

	// Get absolute paths so validation cannot be bypassed by using relative paths
	absSource, err := filepath.Abs(source)
	if err != nil {
		*errs = errors.Join(*errs, fmt.Errorf("%w: error getting absolute path: %w", ErrorInvalidSource, err))
	}
	absDest, err := filepath.Abs(destination)
	if err != nil {
		err = fmt.Errorf("%w: error getting absolute path: %w", ErrorInvalidDestination, err)
		*errs = errors.Join(*errs, err)
	}

	// Make sure source and destination are different
	if absSource == absDest {
		err = fmt.Errorf("%w: source and destination paths cannot be the same", ErrorInvalidSource)
		*errs = errors.Join(*errs, err)
		err = fmt.Errorf("%w: destination and source paths cannot be the same", ErrorInvalidDestination)
		*errs = errors.Join(*errs, err)
	}

	// Make sure destination is not inside of source
	relPath, err := filepath.Rel(absSource, absDest)
	if err != nil {
		err := fmt.Errorf("%w: error checking relative path from source to destination: %w", ErrorInvalidDestination, err)
		*errs = errors.Join(*errs, err)
	}
	if !filepath.IsAbs(relPath) && !strings.HasPrefix(relPath, "..") && relPath != "." {
		err := fmt.Errorf("%w: destination path cannot be inside the source path", ErrorInvalidDestination)
		*errs = errors.Join(*errs, err)
	}
}
