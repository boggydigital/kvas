package kvas

import (
	"bytes"
	"fmt"
	"github.com/boggydigital/nod"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type keyValues struct {
	dir      string
	ext      string
	idx      index
	mtx      *sync.Mutex
	connTime int64
}

const (
	JsonExt = ".json"
	GobExt  = ".gob"
	HtmlExt = ".html"
	XmlExt  = ".xml"
)

const dirPerm os.FileMode = 0755

func NewKeyValues(dir string, ext string) (KeyValues, error) {

	switch ext {
	case JsonExt:
		fallthrough
	case GobExt:
		fallthrough
	case HtmlExt:
		fallthrough
	case XmlExt:
		//do nothing
	default:
		return nil, fmt.Errorf("unknown extension %s", ext)
	}

	kv := &keyValues{
		dir: dir,
		ext: ext,
		idx: make(index),
		mtx: &sync.Mutex{},
	}
	err := kv.idx.read(kv.dir)

	kv.connTime = time.Now().Unix()

	return kv, err
}

// Has verifies if a value set contains provided key
func (lkv *keyValues) Has(key string) bool {
	lkv.mtx.Lock()
	defer lkv.mtx.Unlock()

	_, ok := lkv.idx[key]
	return ok
}

func (lkv *keyValues) Get(key string) (io.ReadCloser, error) {
	if !lkv.Has(key) {
		return nil, nil
	}
	return lkv.GetFromStorage(key)
}

func (lkv *keyValues) GetFromStorage(key string) (io.ReadCloser, error) {
	valAbsPath := lkv.valuePath(key)
	if _, err := os.Stat(valAbsPath); os.IsNotExist(err) {
		return nil, nil
	}
	return os.Open(valAbsPath)
}

func (lkv *keyValues) valuePath(key string) string {
	key = url.PathEscape(key)
	return filepath.Join(lkv.dir, key+lkv.ext)
}

// Set stores a bytes slice value by a provided key
func (lkv *keyValues) Set(key string, reader io.Reader) error {

	var buf bytes.Buffer
	tr := io.TeeReader(reader, &buf)

	// check if value already exists and has the same hash
	hash, err := Sha256(tr)
	if err != nil {
		return err
	}

	lkv.mtx.Lock()

	if hash == lkv.idx[key].Hash {
		lkv.mtx.Unlock()
		return nil
	}

	lkv.mtx.Unlock()

	// write value
	valuePath := lkv.valuePath(key)

	if _, err := os.Stat(lkv.dir); os.IsNotExist(err) {
		if err := os.MkdirAll(lkv.dir, dirPerm); err != nil {
			return err
		}
	}
	file, err := os.Create(valuePath)
	defer file.Close()
	if err != nil {
		return err
	}

	if _, err = io.Copy(file, &buf); err != nil {
		return err
	}

	lkv.mtx.Lock()
	defer lkv.mtx.Unlock()

	// update index
	lkv.idx.upd(key, hash)
	return lkv.idx.write(lkv.dir)
}

// Cut deletes value from keyValues by a provided key
func (lkv *keyValues) Cut(key string) (bool, error) {

	if !lkv.Has(key) {
		return false, nil
	}

	// delete value
	valuePath := lkv.valuePath(key)
	if _, err := os.Stat(valuePath); os.IsNotExist(err) {
		return false, fmt.Errorf("index contains key %s, file not found", key)
	}

	if err := os.Remove(valuePath); err != nil {
		return false, err
	}

	lkv.mtx.Lock()
	defer lkv.mtx.Unlock()

	// update index
	delete(lkv.idx, key)

	return true, lkv.idx.write(lkv.dir)
}

func (lkv *keyValues) Keys() []string {
	return lkv.idx.Keys(lkv.mtx)
}

// CreatedAfter returns keys of values created on or after provided timestamp
func (lkv *keyValues) CreatedAfter(timestamp int64) []string {
	return lkv.idx.CreatedAfter(timestamp, lkv.mtx)
}

// ModifiedAfter returns keys of values modified on or after provided timestamp
// that were created earlier
func (lkv *keyValues) ModifiedAfter(timestamp int64, strictlyModified bool) []string {
	return lkv.idx.ModifiedAfter(timestamp, strictlyModified, lkv.mtx)
}

func (lkv *keyValues) IsModifiedAfter(key string, timestamp int64) bool {
	return lkv.idx.IsModifiedAfter(key, timestamp, lkv.mtx)
}

func (lkv *keyValues) IndexCurrentModTime() (int64, error) {
	indexPath := indexPath(lkv.dir)
	if stat, err := os.Stat(indexPath); os.IsNotExist(err) {
		return -1, nil
	} else if err != nil {
		return -1, err
	} else {
		return stat.ModTime().Unix(), nil
	}
}

func (lkv *keyValues) CurrentModTime(key string) (int64, error) {
	valuePath := lkv.valuePath(key)
	if stat, err := os.Stat(valuePath); os.IsNotExist(err) {
		return -1, nil
	} else if err != nil {
		return -1, err
	} else {
		return stat.ModTime().Unix(), nil
	}
}

func (lkv *keyValues) IndexRefresh() error {
	indexModTime, err := lkv.IndexCurrentModTime()
	if err != nil {
		return err
	}

	lkv.mtx.Lock()
	defer lkv.mtx.Unlock()

	if lkv.connTime < indexModTime {
		if err := lkv.idx.read(lkv.dir); err != nil {
			return err
		}
		lkv.connTime = indexModTime
	}

	return nil
}

func (lkv *keyValues) VetIndexOnly(fix bool, tpw nod.TotalProgressWriter) ([]string, error) {
	indexOnly := make([]string, 0)
	indexModified := false

	keys := lkv.Keys()

	if tpw != nil {
		tpw.TotalInt(len(keys))
	}

	for _, key := range keys {
		valAbsPath := lkv.valuePath(key)
		if _, err := os.Stat(valAbsPath); err == nil {
			if tpw != nil {
				tpw.Increment()
			}
			continue
		}
		indexOnly = append(indexOnly, key)
		if fix {
			delete(lkv.idx, key)
			indexModified = true
		}
		if tpw != nil {
			tpw.Increment()
		}
	}

	if indexModified {
		if err := lkv.idx.write(lkv.dir); err != nil {
			return nil, err
		}
	}

	return indexOnly, nil
}

func (lkv *keyValues) VetIndexMissing(fix bool, tpw nod.TotalProgressWriter) ([]string, error) {
	indexMissing := make([]string, 0)

	filenames, err := filepath.Glob("*" + lkv.ext)
	if err != nil {
		return nil, err
	}

	if tpw != nil {
		tpw.TotalInt(len(filenames))
	}

	for _, fn := range filenames {
		key := strings.TrimSuffix(fn, lkv.ext)
		if _, ok := lkv.idx[key]; !ok {
			indexMissing = append(indexMissing, key)
			if fix {
				valAbsPath := lkv.valuePath(key)
				if err := openSetValue(key, valAbsPath, lkv); err != nil {
					return nil, err
				}
			}
			if tpw != nil {
				tpw.Increment()
			}
		}
	}

	return indexMissing, nil
}

func openSetValue(key, path string, lkv *keyValues) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := lkv.Set(key, f); err != nil {
		return err
	}

	return nil
}
