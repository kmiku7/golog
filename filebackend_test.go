package golog

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
	"time"
)

func createFileBackend(t *testing.T) *FileBackend {
	tempDir, err := ioutil.TempDir("", "fileBackend_test")
	if err != nil {
		t.Fatalf("create temporary directoey failed, err: %v", err)
	}
	fileBackend, err := NewFileBackend(path.Join(tempDir, "log"))
	if err != nil {
		t.Fatalf("create file backend failed, err: %v", err)
	}
	return fileBackend
}

func TestTruncateToHour(t *testing.T) {
	timeEdge := time.Date(2019, 1, 2, 12, 0, 0, 0, time.UTC)
	timePoint := timeEdge.Add(time.Minute * 13)
	timeTruncated := truncateToHour(timePoint)
	if timeTruncated != timeEdge {
		t.Fatalf("expected: %v, return: %v",
			timeEdge.String(), timeTruncated.String())
	}
}

func TestSetFlushInterval(t *testing.T) {
	fileBackend := createFileBackend(t)
	defer fileBackend.Close()

	fileBackend.SetFlushInterval(time.Minute)
	if fileBackend.flushInterval != time.Minute {
		t.Errorf("set flush interval failed, actual: %v", fileBackend.flushInterval.String())
	}
}

func TestLogOutput(t *testing.T) {
	fileBackend := createFileBackend(t)

	outputContent := map[Level]string{
		Debug:   "This is a debug string",
		Info:    "This is a info string.",
		Warning: "This is a warning string.",
		Error:   "This is a error string.",
		Fatal:   "This is a fatal string.",
	}
	for level, content := range outputContent {
		fileBackend.Log(level, []byte(content))
	}
	fileBackend.Close()

	for level, expectContent := range outputContent {
		logFilePath := path.Join(fileBackend.dir, levelNames[level]+logFileSuffix)
		content, err := ioutil.ReadFile(logFilePath)
		if err != nil {
			t.Fatalf("read %s log failed, err: %v", levelNames[level], err)
		}
		if strings.TrimSpace(string(content)) != expectContent {
			t.Errorf("%s log not match, expect: %s, write: %s",
				levelNames[level], expectContent, content)
		}
	}
}

func TestMonitorDaemon(t *testing.T) {
	fileBackend := createFileBackend(t)
	defer fileBackend.Close()

	outputContent := "This is one string."
	for level := range levelNames {
		fileBackend.Log(level, []byte(outputContent))
	}

	files, err := ioutil.ReadDir(fileBackend.dir)
	if err != nil {
		t.Fatalf("read temporary directory failed, err: %v", err)
	}
	if len(files) != levelCount {
		t.Fatalf("count of log file should be %v, actual: %v",
			levelCount, len(files))
	}
	bakSuffix := ".bak"
	for _, file := range files {
		filePath := path.Join(fileBackend.dir, file.Name())
		_, err := ioutil.ReadFile(filePath)
		if err != nil {
			t.Fatalf("read %s content failed, err: %v", file.Name(), err)
		}

		bakFilePath := filePath + bakSuffix
		if err := os.Rename(filePath, bakFilePath); err != nil {
			t.Fatalf("move %s failed, err: %v", file.Name(), err)
		}
	}
	fileBackend.doMonitorFiles()

	for level := range levelNames {
		fileBackend.Log(level, []byte(outputContent))
	}
	fileBackend.Flush()
	files, err = ioutil.ReadDir(fileBackend.dir)
	if err != nil {
		t.Fatalf("read temporary directory failed, err: %v", err)
	}
	if len(files) != levelCount*2 {
		t.Fatalf("count of log file should be %v, actual: %v",
			levelCount*2, len(files))
	}
	for _, file := range files {
		filePath := path.Join(fileBackend.dir, file.Name())
		content, err := ioutil.ReadFile(filePath)
		if err != nil {
			t.Fatalf("read %s content failed, err: %v", file.Name(), err)
		}
		if !(strings.HasSuffix(file.Name(), logFileSuffix) ||
			strings.HasSuffix(file.Name(), logFileSuffix+bakSuffix)) {
			t.Fatalf("invalid file name: %v", file.Name())
		}
		if strings.TrimSpace(string(content)) != outputContent {
			t.Errorf("%s log not match, expect: %s, write: %s",
				file.Name(), outputContent, content)
		}
	}
}

func TestRoratedFilenamePattern(t *testing.T) {
	filename := "DEBUG.log.2019061012"
	matchedString := rotatedFilenamePattern.FindString(filename)
	if filename != matchedString {
		t.Errorf("actual: %v, expect: %v", matchedString, filename)
	}
}

func TestShouldDelete(t *testing.T) {
	fileBackend := createFileBackend(t)
	defer fileBackend.Close()
	timePoint := time.Date(2019, 1, 2, 3, 4, 0, 0, time.UTC)
	filename := fmt.Sprintf("DEBUG.log.%s", timePoint.Format(datetimeSuffixLayout))
	fileBackend.getNowTime = func() time.Time {
		return timePoint.Add(time.Hour * 2)
	}
	if !fileBackend.shouldDelete(filename, 1) {
		t.Errorf("should be deleted")
	}
}

func TestRotate(t *testing.T) {
	fileBackend := createFileBackend(t)
	defer fileBackend.Close()

	nowTime := time.Date(
		2019, 7, 10,
		1, 13, 14, 0,
		time.UTC)
	fileBackend.getNowTime = func() time.Time {
		return nowTime
	}
	fileBackend.SetRotateFile(true, 1)

	// write message into current log file.
	outputContent := "This is one string."
	for level := range levelNames {
		fileBackend.Log(level, []byte(outputContent))
	}

	files, err := ioutil.ReadDir(fileBackend.dir)
	if err != nil {
		t.Fatalf("read temporary directory failed, err: %v", err)
	}
	if len(files) != levelCount {
		t.Fatalf("count of log file should be %v, actual: %v",
			levelCount, len(files))
	}

	// trigger rotate.
	// one current used file, and one rotated file.
	nowTime = nowTime.Add(time.Hour)
	fileBackend.doRotateByHour()
	fileBackend.doMonitorFiles()

	for level := range levelNames {
		fileBackend.Log(level, []byte(outputContent))
	}
	fileBackend.Flush()

	// trigger rotate again.
	// one current used file, and two rotated file.
	// and we set keep two hours log. So the first rotated file will be removed.
	nowTime = nowTime.Add(time.Hour)
	fileBackend.doRotateByHour()
	fileBackend.doMonitorFiles()

	for level := range levelNames {
		fileBackend.Log(level, []byte(outputContent))
	}
	fileBackend.Flush()

	files, err = ioutil.ReadDir(fileBackend.dir)
	if err != nil {
		t.Fatalf("read temporary directory failed, err: %v", err)
	}
	if len(files) != levelCount*2 {
		t.Errorf("count of log file should be %v, actual: %v",
			levelCount*2, len(files))
	}
	timeSuffix := nowTime.Format(datetimeSuffixLayout)
	for _, file := range files {
		filePath := path.Join(fileBackend.dir, file.Name())
		content, err := ioutil.ReadFile(filePath)
		if err != nil {
			t.Errorf("read %s content failed, err: %v", file.Name(), err)
		}
		if !(strings.HasSuffix(file.Name(), logFileSuffix) ||
			strings.HasSuffix(file.Name(), logFileSuffix+"."+timeSuffix)) {
			t.Errorf("invalid file name: %v", file.Name())
		}
		if strings.TrimSpace(string(content)) != outputContent {
			t.Errorf("%s log not match, expect: %s, write: %s",
				file.Name(), outputContent, content)
		}
	}
}
