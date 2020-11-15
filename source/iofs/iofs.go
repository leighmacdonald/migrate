// +build go1.16

package iofs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strconv"

	"github.com/golang-migrate/migrate/v4/source"
)

// Driver is a source driver that wraps io/fs#FS.
type Driver struct {
	migrations *source.Migrations
	fsys       fs.FS
	path       string
}

// NewDriver returns a new Driver from io/fs#FS and a relative path.
func NewDriver(fsys fs.FS, path string) (source.Driver, error) {
	var i Driver
	if err := i.Init(fsys, path); err != nil {
		return nil, fmt.Errorf("failed to init driver with path %s: %w", path, err)
	}
	return &i, nil
}

// Open is part of source.Driver interface implementation.
// Open panics when called directly.
func (i *Driver) Open(url string) (source.Driver, error) {
	panic("iofs: Driver does not support open with url")
}

// Init prepares not initialized IoFS instance to read migrations from a
// io/fs#FS instance and a relative path.
func (i *Driver) Init(fsys fs.FS, path string) error {
	entries, err := fs.ReadDir(fsys, path)
	if err != nil {
		return err
	}

	ms := source.NewMigrations()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m, err := source.DefaultParse(e.Name())
		if err != nil {
			continue
		}
		file, err := e.Info()
		if err != nil {
			continue
		}
		if !ms.Append(m) {
			return source.ErrDuplicateMigration{
				Migration: *m,
				FileInfo:  file,
			}
		}
	}

	i.fsys = fsys
	i.path = path
	i.migrations = ms
	return nil
}

// Close is part of source.Driver interface implementation.
// Closes the file system if possible.
func (d *Driver) Close() error {
	c, ok := d.fsys.(io.Closer)
	if !ok {
		return nil
	}
	return c.Close()
}

// First is part of source.Driver interface implementation.
func (i *Driver) First() (version uint, err error) {
	if version, ok := i.migrations.First(); ok {
		return version, nil
	}
	return 0, &fs.PathError{
		Op:   "first",
		Path: i.path,
		Err:  fs.ErrNotExist,
	}
}

// Prev is part of source.Driver interface implementation.
func (i *Driver) Prev(version uint) (prevVersion uint, err error) {
	if version, ok := i.migrations.Prev(version); ok {
		return version, nil
	}
	return 0, &fs.PathError{
		Op:   "prev for version " + strconv.FormatUint(uint64(version), 10),
		Path: i.path,
		Err:  fs.ErrNotExist,
	}
}

// Next is part of source.Driver interface implementation.
func (i *Driver) Next(version uint) (nextVersion uint, err error) {
	if version, ok := i.migrations.Next(version); ok {
		return version, nil
	}
	return 0, &fs.PathError{
		Op:   "next for version " + strconv.FormatUint(uint64(version), 10),
		Path: i.path,
		Err:  fs.ErrNotExist,
	}
}

// ReadUp is part of source.Driver interface implementation.
func (i *Driver) ReadUp(version uint) (r io.ReadCloser, identifier string, err error) {
	if m, ok := i.migrations.Up(version); ok {
		body, err := i.open(path.Join(i.path, m.Raw))
		if err != nil {
			return nil, "", err
		}
		return body, m.Identifier, nil
	}
	return nil, "", &fs.PathError{
		Op:   "read up for version " + strconv.FormatUint(uint64(version), 10),
		Path: i.path,
		Err:  fs.ErrNotExist,
	}
}

// ReadDown is part of source.Driver interface implementation.
func (i *Driver) ReadDown(version uint) (r io.ReadCloser, identifier string, err error) {
	if m, ok := i.migrations.Down(version); ok {
		body, err := i.open(path.Join(i.path, m.Raw))
		if err != nil {
			return nil, "", err
		}
		return body, m.Identifier, nil
	}
	return nil, "", &fs.PathError{
		Op:   "read down for version " + strconv.FormatUint(uint64(version), 10),
		Path: i.path,
		Err:  fs.ErrNotExist,
	}
}

func (i *Driver) open(path string) (fs.File, error) {
	f, err := i.fsys.Open(path)
	if err == nil {
		return f, nil
	}
	// Some non-standard file systems may return errors that don't include the path, that
	// makes debugging harder.
	if !errors.As(err, new(*fs.PathError)) {
		err = &fs.PathError{
			Op:   "open",
			Path: path,
			Err:  err,
		}
	}
	return nil, err
}
