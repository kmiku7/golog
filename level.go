package golog

type Level int

const (
	Debug Level = iota
	Info
	Warning
	Error
	Fatal
	levelCount int = iota
)

const (
	levelMin Level = Debug
	levelMax Level = Fatal
)

var (
	levelNames = map[Level]string{
		Debug:   "DEBUG",
		Info:    "INFO",
		Warning: "WARNING",
		Error:   "ERROR",
		Fatal:   "FATAL",
	}
)
