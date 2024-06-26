package kvas

import (
	"bytes"
	"github.com/boggydigital/testo"
	"golang.org/x/exp/slices"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func mockLocalKeyValues() *keyValues {
	return &keyValues{
		dir:      os.TempDir(),
		ext:      GobExt,
		idx:      mockIndex(),
		mtx:      &sync.Mutex{},
		connTime: time.Now().Unix(),
	}
}

func cleanupLocalKeyValues(kv *keyValues) error {
	if kv == nil {
		return nil
	}
	for id := range kv.idx {
		path := filepath.Join(kv.dir, id+kv.ext)
		if _, err := os.Stat(path); err == nil {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}

	return nil
}

func TestConnectLocal(t *testing.T) {
	tests := []struct {
		ext    string
		expNil bool
		expErr bool
	}{
		{"", true, true},
		{".txt", true, true},
		{"json", true, true},
		{"gob", true, true},
		{JsonExt, false, false},
		{GobExt, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			lkv, err := NewKeyValues(os.TempDir(), tt.ext)
			testo.Nil(t, lkv, tt.expNil)
			testo.Error(t, err, tt.expErr)

			testo.Error(t, indexCleanup(), false)
		})
	}
}

func TestLocalKeyValuesSetHasGetCut(t *testing.T) {
	tests := []struct {
		set []string
		get map[string]bool
	}{
		{nil, nil},
		{[]string{"x1", "x1"}, map[string]bool{"x1": false}},
		{[]string{"y1", "y2"}, map[string]bool{"y1": false, "y2": false, "y3": true}},
	}

	for ii, tt := range tests {
		t.Run(strconv.Itoa(ii), func(t *testing.T) {
			lkv, err := NewKeyValues(os.TempDir(), GobExt)
			testo.Nil(t, lkv, false)
			testo.Error(t, err, false)

			// Set, Has tests
			for _, sk := range tt.set {
				err = lkv.Set(sk, strings.NewReader(sk))
				testo.Error(t, err, false)
				testo.EqualValues(t, lkv.Has(sk), true)
			}

			// Get tests
			for gk, expNil := range tt.get {
				rc, err := lkv.Get(gk)
				testo.Error(t, err, false)
				testo.Nil(t, rc, expNil)

				if expNil {
					continue
				}

				var val []byte
				buf := bytes.NewBuffer(val)
				num, err := io.Copy(buf, rc)
				testo.EqualValues(t, num, int64(len(gk)))
				testo.EqualValues(t, gk, buf.String())

				testo.Error(t, rc.Close(), false)
			}

			// Cut, Has tests

			for _, ck := range tt.set {
				has := lkv.Has(ck)
				ok, err := lkv.Cut(ck)
				testo.EqualValues(t, ok, has)
				testo.Error(t, err, false)
			}

			testo.Error(t, indexCleanup(), false)
		})
	}
}

func TestLocalKeyValues_CreatedAfter(t *testing.T) {

	tests := []struct {
		after int64
		exp   []string
	}{
		{-1, []string{"1", "2", "3"}},
		{0, []string{"1", "2", "3"}},
		{1, []string{"1", "2", "3"}},
		{2, []string{"2", "3"}},
		{3, []string{"3"}},
		{4, []string{}},
	}

	kv := mockLocalKeyValues()
	for ii, tt := range tests {
		t.Run(strconv.Itoa(ii), func(t *testing.T) {
			ca := kv.CreatedAfter(tt.after)
			testo.EqualValues(t, len(ca), len(tt.exp))
			for _, cav := range ca {
				testo.EqualValues(t, slices.Contains(tt.exp, cav), true)
			}
		})
	}
}

func TestLocalKeyValues_ModifiedAfter(t *testing.T) {

	tests := []struct {
		after int64
		sm    bool
		exp   []string
	}{
		{-1, false, []string{"1", "2", "3"}},
		{0, false, []string{"1", "2", "3"}},
		{1, false, []string{"1", "2", "3"}},
		{2, false, []string{"2", "3"}},
		{3, false, []string{"3"}},
		{4, false, []string{}},
		{-1, true, []string{}},
		{0, true, []string{}},
		{1, true, []string{}},
		{2, true, []string{}},
		{3, true, []string{}},
		{4, true, []string{}},
	}

	kv := mockLocalKeyValues()
	for ii, tt := range tests {
		t.Run(strconv.Itoa(ii), func(t *testing.T) {
			ma := kv.ModifiedAfter(tt.after, tt.sm)
			testo.EqualValues(t, len(ma), len(tt.exp))
			for _, mav := range ma {
				testo.EqualValues(t, slices.Contains(tt.exp, mav), true)
			}
		})
	}
}

func TestLocalKeyValues_IsModifiedAfter(t *testing.T) {

	tests := []struct {
		key   string
		after int64
		exp   bool
	}{
		{"1", -1, true},
		{"1", 0, true},
		{"1", 1, false},
		{"1", 2, false},
		{"2", 0, true},
		{"2", 1, true},
		{"2", 2, false},
	}

	kv := mockLocalKeyValues()
	for ii, tt := range tests {
		t.Run(strconv.Itoa(ii), func(t *testing.T) {
			testo.EqualValues(t, kv.IsModifiedAfter(tt.key, tt.after), tt.exp)
		})
	}
}

func TestLocalKeyValues_IndexCurrentModTime(t *testing.T) {
	start := time.Now().Unix()

	kv := mockLocalKeyValues()
	testo.Error(t, kv.idx.write(os.TempDir()), false)

	imt, err := kv.IndexCurrentModTime()
	testo.Error(t, err, false)
	testo.CompareInt64(t, imt, start, testo.GreaterOrEqual)

	testo.Error(t, indexCleanup(), false)
}

func TestLocalKeyValues_CurrentModTime(t *testing.T) {
	start := time.Now().Unix()

	kv := mockLocalKeyValues()

	testo.Error(t, kv.Set("test", strings.NewReader("test")), false)

	cmt, err := kv.CurrentModTime("1")
	testo.Error(t, err, false)
	testo.CompareInt64(t, cmt, start, testo.Less)

	cmt, err = kv.CurrentModTime("test")
	testo.Error(t, err, false)
	testo.CompareInt64(t, cmt, start, testo.GreaterOrEqual)

	cmt, err = kv.CurrentModTime("2")
	testo.Error(t, err, false)
	testo.CompareInt64(t, cmt, start, testo.Less)

	testo.Error(t, cleanupLocalKeyValues(kv), false)
}

func TestLocalKeyValues_IndexRefresh(t *testing.T) {
	kv := mockLocalKeyValues()
	testo.Error(t, kv.idx.write(os.TempDir()), false)

	// first test: reset connection time, clear index
	// expected result: index refresh will reload, setting connection time and index

	kv.idx = make(index)
	kv.connTime = 0
	testo.EqualValues(t, len(kv.idx), 0)

	testo.Error(t, kv.IndexRefresh(), false)
	testo.CompareInt64(t, kv.connTime, 0, testo.Greater)
	testo.CompareInt64(t, int64(len(kv.idx)), 0, testo.Greater)

	// second test: clear index, but don't reset connection time
	// expected result: index refresh won't do anything and index will remain empty
	// because connection time didn't changed and there's no need to reload index

	kv.idx = make(index)
	testo.EqualValues(t, len(kv.idx), 0)

	testo.Error(t, kv.IndexRefresh(), false)
	testo.EqualValues(t, len(kv.idx), 0)

	testo.Error(t, indexCleanup(), false)
}
