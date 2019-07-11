package golog

import (
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
	fileBackend.SetRotateFile(true, 10)

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
		t.Fatalf("count of log file should be %v, actual: %v",
			levelCount*2, len(files))
	}
	timeSuffix := nowTime.Format(datetimeSuffixLayout)
	for _, file := range files {
		filePath := path.Join(fileBackend.dir, file.Name())
		content, err := ioutil.ReadFile(filePath)
		if err != nil {
			t.Fatalf("read %s content failed, err: %v", file.Name(), err)
		}
		if !(strings.HasSuffix(file.Name(), logFileSuffix) ||
			strings.HasSuffix(file.Name(), logFileSuffix+"."+timeSuffix)) {
			t.Fatalf("invalid file name: %v", file.Name())
		}
		if strings.TrimSpace(string(content)) != outputContent {
			t.Errorf("%s log not match, expect: %s, write: %s",
				file.Name(), outputContent, content)
		}
	}

}
