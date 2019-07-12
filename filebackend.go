package golog

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultFlushInterval = time.Second * 3
	defaultBufferSize    = 256 * 1024
	datetimeSuffixLayout = "2006010215"
	logFileSuffix        = ".log"
)

var (
	rotatedFilenamePattern *regexp.Regexp
)

func init() {
	names := make([]string, 0, len(levelNames))
	for _, name := range levelNames {
		names = append(names, name)
	}
	rotatedFilenamePattern = regexp.MustCompile(fmt.Sprintf(
		"(%s)\\.log\\.20[0-9]{8}", strings.Join(names, "|")))
}

func truncateToHour(t time.Time) time.Time {
	return t.Truncate(time.Hour)
}

type syncBufio struct {
	writer    *bufio.Writer
	file      *os.File
	writeSize uint64
	filePath  string
}

func newSyncBufio(file *os.File, filepath string, bufferSize int) *syncBufio {
	return &syncBufio{
		writer:   bufio.NewWriterSize(file, bufferSize),
		file:     file,
		filePath: filepath,
	}
}

func (s *syncBufio) flush() error {
	return s.writer.Flush()
}

func (s *syncBufio) sync() error {
	return s.file.Sync()
}

func (s *syncBufio) close() error {
	if err := s.flush(); err != nil {
		fmt.Fprintf(os.Stderr, "flush failed: %v", err)
	}
	if err := s.sync(); err != nil {
		fmt.Fprintf(os.Stderr, "sync failed: %v", err)
	}
	return s.file.Close()
}

func (s *syncBufio) write(content []byte) {
	writeCount, err := s.writer.Write(content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write file failed: %v", err)
	}
	s.writeSize += uint64(writeCount)
}

type FileBackend struct {
	mutex          sync.Mutex
	dir            string
	writer         [levelCount]*syncBufio
	flushInterval  time.Duration
	rotateByHour   bool
	lastRotateTime int64
	keepHours      int

	rotatedFilenamePattern *regexp.Regexp
	getNowTime             func() time.Time
}

func NewFileBackend(dir string) (*FileBackend, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	var fileBackend FileBackend
	fileBackend.dir = dir
	fileBackend.flushInterval = defaultFlushInterval
	fileBackend.rotatedFilenamePattern = rotatedFilenamePattern
	fileBackend.getNowTime = time.Now

	for i := levelMin; i <= levelMax; i++ {
		filepath := path.Join(dir, levelNames[i]+logFileSuffix)
		if err := fileBackend.openSyncBufio(i, filepath); err != nil {
			return nil, err
		}
	}

	intervalLoop := func(f func(), d time.Duration) {
		for {
			time.Sleep(d)
			f()
		}
	}

	go intervalLoop((&fileBackend).Flush, fileBackend.flushInterval)
	go intervalLoop((&fileBackend).doMonitorFiles, time.Second*5)
	go intervalLoop((&fileBackend).doRotateByHour, time.Second*1)

	return &fileBackend, nil
}

func (s *FileBackend) openSyncBufio(level Level, filepath string) error {
	file, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	s.writer[level] = newSyncBufio(file, filepath, defaultBufferSize)
	return nil
}

func (s *FileBackend) SetRotateFile(rotateByHour bool, keepHours int) {
	s.rotateByHour = rotateByHour
	if rotateByHour {
		s.keepHours = keepHours
		s.lastRotateTime = truncateToHour(s.getNowTime()).Unix()
	} else {
		s.lastRotateTime = 0
	}
}

func (s *FileBackend) SetFlushInterval(t time.Duration) {
	s.flushInterval = t
}

func (s *FileBackend) doRotateByHour() {
	if !s.rotateByHour {
		return
	}

	// rotate files
	rotateTime := truncateToHour(s.getNowTime())
	ru := rotateTime.Unix()
	_ = ru
	if rotateTime.Unix() > s.lastRotateTime {
		for i := levelMin; i <= levelMax; i++ {
			originalFilename := s.writer[i].filePath
			newFilename := originalFilename + "." + rotateTime.Format(datetimeSuffixLayout)
			os.Rename(originalFilename, newFilename)
		}
	}

	// remove old files
	if s.keepHours <= 0 {
		return
	}
	files, err := ioutil.ReadDir(s.dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read dir %s failed: %v", s.dir, err)
		return
	}
	for _, file := range files {
		if file.Name() == s.rotatedFilenamePattern.FindString(file.Name()) &&
			s.shouldDelete(file.Name(), s.keepHours) {
			fullpath := filepath.Join(s.dir, file.Name())
			if err := os.Remove(fullpath); err != nil {
				fmt.Fprintf(os.Stderr, "remove %s failed: %v", fullpath, err)
			}
		}
	}
}

func (s *FileBackend) doMonitorFiles() {
	for i := levelMin; i <= levelMax; i++ {
		if s.writer[i] == nil {
			continue
		}

		writer := s.writer[i]
		filepath := writer.filePath
		_, err := os.Stat(filepath)
		if err == nil {
			return
		}
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "stat %s failed: %v", filepath, err)
			return
		}
		lockerWrapper := func(level Level, filepath string) error {
			s.mutex.Lock()
			defer s.mutex.Unlock()
			return s.openSyncBufio(level, filepath)
		}
		if err := lockerWrapper(i, filepath); err != nil {
			fmt.Fprintf(os.Stderr, "open %s failed: %v", filepath, err)
			return
		}
		writer.close()
	}
}

func (s *FileBackend) flush() {
	for i := 0; i < int(levelCount); i++ {
		if s.writer[i] == nil {
			continue
		}

		s.writer[i].flush()
		s.writer[i].sync()
	}
}

func (s *FileBackend) Flush() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.flush()
}

func (s *FileBackend) close() {
	for i := 0; i < int(levelCount); i++ {
		if err := s.writer[i].close(); err != nil {
			fmt.Fprintf(os.Stderr, "close failed: %v", err)
		}
		s.writer[i] = nil
	}
}

func (s *FileBackend) Close() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.close()
}

func (s *FileBackend) shouldDelete(name string, keepHours int) bool {
	datetimeSuffix := strings.Split(name, ".")[2]
	fileTime, err := time.Parse(datetimeSuffixLayout, datetimeSuffix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse datetime suffix failed, name: %v, err: %v", name, err)
		return false
	}
	fileTime = fileTime.Add(time.Duration(keepHours) * time.Hour)
	removePoint := truncateToHour(s.getNowTime())
	if !fileTime.After(removePoint) {
		return true
	}
	return false
}

func (s *FileBackend) Log(level Level, content []byte) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if level >= levelMin && level <= levelMax {
		s.writer[level].write(content)
	} else {
		fmt.Fprintf(os.Stderr, "invalid level: %v, content: %s", level, content)
	}
	if level == Fatal {
		s.flush()
	}
}
