package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"strings"
	"time"
)

// https://github.com/fsnotify/fsnotify/blob/main/README.md
func setupFileWatcher(paths []string) *fsnotify.Watcher {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				log.WithField("eventName", event).Debug("event")
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.WithField("eventName", event.Name).Debug("modified file")
					readLogFile <- event.Name
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
				log.WithError(err).Error("Unable to watch Factorio log file for changes")
			}
		}
	}()

	for _, path := range paths {
		err = watcher.Add(path)
		if err != nil {
			log.WithError(err).WithField("path", path).Error("Could not watch path/file")
		}
	}
	return watcher
}

func getLoggerFromConfig(logLevel string) *logrus.Logger {
	logLevel = strings.ToLower(logLevel)
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{ForceQuote: true, TimestampFormat: time.RFC3339Nano})

	switch logLevel {
	case "debug":
		log.Level = logrus.DebugLevel
	case "info":
		log.Level = logrus.InfoLevel
	case "warning":
		log.Level = logrus.WarnLevel
	case "fatal":
		log.Level = logrus.FatalLevel
	default:
		log.Level = logrus.InfoLevel
	}
	return log
}

// Get last line from given file
// https://stackoverflow.com/a/51328256/2451037
func getLastLineWithSeek(filepath string) string {
	fileHandle, err := os.Open(filepath)

	if err != nil {
		panic("Cannot open file")
		os.Exit(1)
	}
	defer fileHandle.Close()

	line := ""
	var cursor int64 = 0
	stat, _ := fileHandle.Stat()
	filesize := stat.Size()
	for {
		cursor -= 1
		fileHandle.Seek(cursor, io.SeekEnd)

		char := make([]byte, 1)
		fileHandle.Read(char)

		if cursor != -1 && (char[0] == 10 || char[0] == 13) { // stop if we find a line
			break
		}

		line = fmt.Sprintf("%s%s", string(char), line) // there is more efficient way

		if cursor == -filesize { // stop if we are at the begining
			break
		}
	}

	return line
}
