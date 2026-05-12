package log

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

var logger *log.Logger

func Init() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".rig")
	os.MkdirAll(dir, 0o755)

	f, err := os.OpenFile(filepath.Join(dir, "rig.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	logger = log.New(f, "", log.LstdFlags|log.Lshortfile)
	logger.Println("--- rig started ---")
	return nil
}

func Info(format string, args ...any) {
	if logger != nil {
		logger.Output(2, fmt.Sprintf("[INFO] "+format, args...))
	}
}

func Error(format string, args ...any) {
	if logger != nil {
		logger.Output(2, fmt.Sprintf("[ERROR] "+format, args...))
	}
}
