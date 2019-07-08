package golog

type Level int

const (
	Debug Level = iota
	Info
	Warning
	Error
	Fatal
)