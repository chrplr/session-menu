// go-task is the dummy Go task of the example project: it reads config.toml,
// in the project folder (the parent of bin/), prints a message taken from
// it, then its arguments.
//
// Build it into bin/, from the project folder:
//
//	go build -C go-task -o ../bin/ .
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type config struct {
	Lab              string  `toml:"lab"`
	Greeting         string  `toml:"greeting"`
	ScreenDistanceCm float64 `toml:"screen_distance_cm"`
}

// defaultConfig is config.toml in the parent of the folder of the executable
// (bin/), or in the current directory if that cannot be found.
func defaultConfig() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(filepath.Dir(filepath.Dir(exe)), "config.toml")
}

func main() {
	// -s and -screen are given by command_line_args.txt, -run by the session
	// lines.
	subject := flag.String("s", "", "subject ID")
	screen := flag.Int("screen", 0, "monitor showing the stimuli")
	run := flag.Int("run", 1, "run number")
	configPath := flag.String("config", defaultConfig(), "settings file")
	flag.Parse()

	b, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	var cfg config
	if err := toml.Unmarshal(b, &cfg); err != nil {
		log.Fatalf("%s: %v", *configPath, err)
	}

	fmt.Printf("%s go-task (%s, screen at %v cm)\n", cfg.Greeting, cfg.Lab, cfg.ScreenDistanceCm)
	fmt.Printf("arguments: %q\n", os.Args[1:])
	fmt.Printf("subject %q, screen %d, run %d\n", *subject, *screen, *run)
	fmt.Println("SUBJECT in the environment:", os.Getenv("SUBJECT"))

	fmt.Print("Press Enter to return to the menu...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
