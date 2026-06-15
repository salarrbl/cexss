package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
        "github.com/yourusername/cexss/pkg/logger"
)

func main() {


        log := logger.New(true)

	log.Info("Cexss starting...")
	log.Debug("Debug mode is active")
	log.Warn("This is a warning")
	log.Error("This is an error (goes to stderr)")
	log.Info("Done")

	// 1. Define Command Line Flags
	// flag.String defines a string flag. 
	// Arguments: (name, default value, description)
	urlFlag := flag.String("u", "", "Single target URL")
	fileFlag := flag.String("f", "", "File containing target URLs")

	// 2. Parse the flags provided by the user
	flag.Parse()

	// 3. Create a channel to store our URLs
	// This "conveyor belt" can hold 100 strings before it pauses to wait for someone to take them.
	urlChan := make(chan string, 100)

	// 4. A "Goroutine" to handle input loading
	// We use 'go func()' so the input reading doesn't block the rest of the program.
	go func() {
		// Close the channel when we are done reading everything
		defer close(urlChan)

		// Priority 1: Single URL (-u)
		if *urlFlag != "" {
			urlChan <- *urlFlag
		}

		// Priority 2: File Input (-f)
		if *fileFlag != "" {
			readFile(*fileFlag, urlChan)
		}

		// Priority 3: Standard Input (Piping)
		// We check if the user is piping data (like: cat urls.txt | cexss)
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			readStdin(urlChan)
		}
	}()

	// 5. Temporary: Print URLs from the channel to verify it works
	fmt.Println("[+] Loading targets...")
	for target := range urlChan {
		fmt.Printf("Found target: %s\n", target)
	}
}

// readFile opens a file and sends each line to the channel
func readFile(path string, out chan string) {
	file, err := os.Open(path)
	if err != nil {
		fmt.Printf("[-] Error opening file: %v\n", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			out <- line
		}
	}
}

// readStdin reads lines from the terminal pipe and sends them to the channel
func readStdin(out chan string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			out <- line
		}
	}
}
