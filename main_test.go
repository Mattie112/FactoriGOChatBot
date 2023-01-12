package main

import (
	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"reflect"
	"testing"
	"time"
)

func Test_parseAndFormatMessage(t *testing.T) {
	// Initialize and consume on 2 channels so we don't block on the channel insert (and therefore sleep al goroutines)
	// There is probaly a better wayo to do this
	commands = make(chan string)
	go func() {
		for range commands {
		}
	}()
	discordActivities = make(chan discordgo.Activity)
	go func() {
		for range discordActivities {
		}
	}()
	type args struct {
		message string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"JOIN", args{message: "2022-02-01 15:31:19 [JOIN] Mattie joined the game"}, ":green_circle: | `Mattie` joined the game!"},
		{"LEAVE", args{message: "2022-02-01 15:31:30 [LEAVE] Mattie left the game"}, ":red_circle: | `Mattie` left the game!"},
		{"CHAT", args{message: "2022-02-01 15:31:30 [CHAT] Mattie: Some chat message"}, ":speech_left: | `Mattie`: Some chat message"},
		// Messages below are generated by the companion mod, but I still want them to go through the normal flow!
		{"PLAYER_DIED", args{message: "[FactoriGOChatBot]: \"2852569 [PLAYER_DIED:Mattie]\""}, ":skull: | Player died: `Mattie` (unknown cause)"},
		// Player died messages
		{"PLAYER_DIED_CAUSE", args{message: "[FactoriGOChatBot]: \"2852569 [PLAYER_DIED:Mattie:locomotive]\""}, ":skull: | Player died: `Mattie`, cause: `locomotive` (hahaha!)"},
		{"PLAYER_DIED_CAUSE", args{message: "[FactoriGOChatBot]: \"2852569 [PLAYER_DIED:Mattie:cargo-wagon]\""}, ":skull: | Player died: `Mattie`, cause: `cargo-wagon` (hahaha! how the hell did you do that?!?!)"},
		{"PLAYER_DIED_CAUSE", args{message: "[FactoriGOChatBot]: \"2852569 [PLAYER_DIED:Mattie:big worm]\""}, ":skull: | Player died: `Mattie`, cause: `big worm`"},
		{"PLAYER_DIED_CAUSE", args{message: "[[FactoriGOChatBot]: \"11083545 [PLAYER_DIED:Vance307:behemoth-spitter]\""}, ":skull: | Player died: `Vance307`, cause: `behemoth-spitter`"},
		// Player died (with counts, companion mod > 0.6.0)
		{"PLAYER_DIED_CAUSE", args{message: "[FactoriGOChatBot]: \"2852569 [PLAYER_DIED:Mattie:locomotive:10:50]\""}, ":skull: | Player died: `Mattie`, cause: `locomotive` (hahaha!) (10 times out of 50 deaths)"},
		{"PLAYER_DIED_CAUSE", args{message: "[FactoriGOChatBot]: \"2852569 [PLAYER_DIED:Mattie:cargo-wagon:10:50]\""}, ":skull: | Player died: `Mattie`, cause: `cargo-wagon` (hahaha! how the hell did you do that?!?!) (10 times out of 50 deaths)"},
		{"PLAYER_DIED_CAUSE", args{message: "[FactoriGOChatBot]: \"2852569 [PLAYER_DIED:Vance307:behemoth-spitter:1:1]\""}, ":skull: | Player died: `Vance307`, cause: `behemoth-spitter` (1 times out of 1 deaths)"},
		{"PLAYER_DIED_CAUSE", args{message: "[FactoriGOChatBot]: \"2852569 [PLAYER_DIED:Mattie:character:1:2]\""}, ":skull: | Player died: `Mattie`, cause: `character` (1 times out of 2 deaths)"},
		{"PLAYER_DIED_CAUSE", args{message: "[FactoriGOChatBot]: \"36777 [PLAYER_DIED:Mattie:small-biter:0:4]\""}, ":skull: | Player died: `Mattie`, cause: `small-biter` (0 times out of 4 deaths)"},
		{"PLAYER_DIED_CAUSE", args{message: "[FactoriGOChatBot]: \"13934261 [PLAYER_DIED:Mattie::1:15]\""}, ":skull: | Player died: `Mattie`, cause: `unknown` (1 times out of 15 deaths)"},
		{"PLAYER_DIED_CAUSE", args{message: "[FactoriGOChatBot]: \"13934261 [PLAYER_DIED:Mattie:SomeOtherPlayerNickNameHere:1:15]\""}, ":skull: | Player died: `Mattie`, cause: `SomeOtherPlayerNickNameHere` (1 times out of 15 deaths)"},
		// Research
		{"RESEARCH_STARTED", args{message: "[FactoriGOChatBot]: \"3045105 [RESEARCH_STARTED:nuclear-power]\""}, ":microscope: | Research started: `nuclear-power`"},
		{"RESEARCH_FINISHED", args{message: "[FactoriGOChatBot]: \"3229214 [RESEARCH_FINISHED:nuclear-power]\""}, ":microscope: | Research finished: `nuclear-power`"},
		// Rocket
		{"ROCKET_LAUNCHED_1", args{message: "[FactoriGOChatBot]: \"12393460 [ROCKET_LAUNCHED:1]\""}, ":rocket: :rocket: :rocket: A rocket has been launched! (1 times)"},
		{"ROCKET_LAUNCHED_5", args{message: "[FactoriGOChatBot]: \"12393460 [ROCKET_LAUNCHED:5]\""}, ":rocket: :rocket: :rocket: A rocket has been launched! (5 times)"},
		{"ROCKET_LAUNCHED_10", args{message: "[FactoriGOChatBot]: \"12393460 [ROCKET_LAUNCHED:10]\""}, ":rocket: :rocket: :rocket: A rocket has been launched! (10 times)"},
		{"ROCKET_LAUNCHED_50", args{message: "[FactoriGOChatBot]: \"12393460 [ROCKET_LAUNCHED:50]\""}, ":rocket: :rocket: :rocket: A rocket has been launched! (50 times)"},
		{"ROCKET_LAUNCHED_100", args{message: "[FactoriGOChatBot]: \"12393460 [ROCKET_LAUNCHED:100]\""}, ":rocket: :rocket: :rocket: A rocket has been launched! (100 times)"},
		{"ROCKET_LAUNCHED_500", args{message: "[FactoriGOChatBot]: \"12393460 [ROCKET_LAUNCHED:500]\""}, ":rocket: :rocket: :rocket: A rocket has been launched! (500 times)"},
		{"ROCKET_LAUNCHED_11", args{message: "[FactoriGOChatBot]: \"12393460 [ROCKET_LAUNCHED:11]\""}, ""},
		{"ROCKET_LAUNCHED_101", args{message: "[FactoriGOChatBot]: \"12393460 [ROCKET_LAUNCHED:101]\""}, ""},
		{"ROCKET_LAUNCHED_220", args{message: "[FactoriGOChatBot]: \"12393460 [ROCKET_LAUNCHED:220]\""}, ""},
		{"ROCKET_LAUNCHED_1337", args{message: "[FactoriGOChatBot]: \"12393460 [ROCKET_LAUNCHED:1337]\""}, ""},
		// Corrupted messages (as I don't know yet how to fix the file read, so it will have a single line guaranteed
		{"CORRUPT", args{message: "[FactoriGOChatBot]: \"2852569 [foobar]\""}, ""},
		// Messages I want to ignore
		{"GPS", args{message: "2022-04-14 19:41:54 [CHAT] Mattie: [gps=98,69]"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseAndFormatMessage(tt.args.message); got != tt.want {
				t.Errorf("parseAndFormatMessage() = '%v', want '%v'", got, tt.want)
			}
		})
	}
	close(commands)
}

func Test_parseDiscordMessage(t *testing.T) {
	type args struct {
		message string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{"Normal chat", args{message: "test"}, []string{"test"}},
		{"Wave emoji", args{message: "👋"}, []string{"**waving-hand**"}},
		{"Sweat", args{message: "😓"}, []string{"**downcast-face-with-sweat**"}},
		{"Grin", args{message: "😀"}, []string{"**grinning-face**"}},
		{"Smile (but slightly)", args{message: "🙂"}, []string{"**slightly-smiling-face**"}},
		{"Cool guy", args{message: "😎"}, []string{"**smiling-face-with-sunglasses**"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseDiscordMessage(tt.args.message); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseDiscordMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_readFactorioLogFile(t *testing.T) {
	log = logrus.New()
	log.Out = io.Discard
	// Prepare channel (readFactorioLogFile will publish to this one)
	messagesToDiscord = make(chan string, 10)

	// First write a file with some lines so that we have a baseline
	d1 := []byte("this\nis\na\n\test\n")
	err := os.WriteFile("test.txt", d1, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Remove("test.txt")
	}()

	// Start the tail process and after some time stop it (so that the program finishes)
	go readFactorioLogFile("test.txt")
	time.Sleep(100 * time.Millisecond) // Give the tail stuff some time to "activate"
	go func() {
		<-time.After(100 * time.Millisecond)
		_ = tailFile.Stop()
	}()

	// Write / append something (e.g. a new line written by factorio)
	f, err := os.OpenFile("test.txt", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}

	write := "2022-02-01 15:31:30 [CHAT] Mattie: Some chat message"
	want := ":speech_left: | `Mattie`: Some chat message"
	if _, err = f.WriteString(write); err != nil {
		panic(err)
	}
	_ = f.Close()

	str := <-messagesToDiscord
	if got := str; !reflect.DeepEqual(got, want) {
		t.Errorf("readFactorioLogFile() = '%v', want '%v'", got, want)
	}
	close(messagesToDiscord)
}
