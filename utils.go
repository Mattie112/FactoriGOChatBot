package main

import (
	"errors"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
)

var reHostname = regexp.MustCompile(`^([a-zA-Z0-9_]([a-zA-Z0-9_-]*[a-zA-Z0-9_])?\.)*[a-zA-Z0-9_]([a-zA-Z0-9_-]*[a-zA-Z0-9_])?$`)

func schedule(delay time.Duration, what func()) chan struct{} {
	ticker := time.NewTicker(delay)
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
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{ForceQuote: true, TimestampFormat: time.RFC3339Nano})

	// Supports: trace, debug, info, warn(ing), error, fatal and panic. Anything else (or nothing) means info
	level, err := logrus.ParseLevel(strings.TrimSpace(logLevel))
	if err != nil {
		level = logrus.InfoLevel
	}
	log.Level = level
	return log
}

func getEnvStrOrDefault(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}

func getEnvOrDefaultBool(key string, defaultVal bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		b, err := strconv.ParseBool(value)
		if err != nil {
			log.WithField("envVar", key).WithError(err).Error("Cannot parse env variable as boolean")
			return defaultVal
		}
		return b
	}
	return defaultVal
}

func validateIpOrHostname(input string) (string, error) {
	// Check if input is a valid IP address
	if net.ParseIP(input) != nil {
		return input, nil
	}

	// Check if input is a valid hostname. Labels may start with a digit and may contain underscores (not valid in
	// DNS, but common for Docker container names). The last label cannot be numeric, that would be a (broken) IP.
	if reHostname.MatchString(input) {
		lastLabel := input[strings.LastIndex(input, ".")+1:]
		if _, err := strconv.Atoi(lastLabel); err != nil {
			return input, nil
		}
	}

	return "", errors.New("input is neither a valid IP address nor a valid hostname")
}

func getEnvIp(key string) (string, error) {
	v := os.Getenv(key)
	return validateIpOrHostname(v)
}

func getEnvInt(key string) (int, error) {
	v := os.Getenv(key)
	i, err := strconv.Atoi(v)
	return i, err
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
