package main

import (
	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
	"os"
	"strconv"
	"strings"
	"time"
)

func schedule(delay time.Duration, what func()) chan struct{} {
	ticker := time.NewTicker(delay * time.Second)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				what()
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
	return quit
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

func getenvStr(key string) (string, error) {
	v := os.Getenv(key)
	return v, nil
}

func getenvBool(key string) bool {
	s, err := getenvStr(key)
	if err != nil {
		log.WithField("envVar", key).WithError(err).Error("Cannot parse env variable as boolean")
		return false
	}
	if s == "" {
		return false // No env var is false
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		log.WithField("envVar", key).WithError(err).Error("Cannot parse env variable as boolean")
		return false
	}
	return v
}

func activityToStatus(activity *discordgo.Activity) string {
	switch activity.Type {
	case discordgo.ActivityTypeGame:
		return "Playing " + activity.Name
	case discordgo.ActivityTypeStreaming:
		return "Streaming " + activity.Name
	case discordgo.ActivityTypeListening:
		return "Listening to " + activity.Name
	case discordgo.ActivityTypeWatching:
		return "Watching " + activity.Name
	case discordgo.ActivityTypeCustom:
		return activity.Emoji.Name + " " + activity.Name
	case discordgo.ActivityTypeCompeting:
		return "Competing in " + activity.Name
	}
	return "Unknown"
}
