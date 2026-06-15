package main

import (
	"bufio"
	"flag"
	"os"

	// Importing our custom logger
	"github.com/salarrbl/cexss/pkg/logger"
)

func main() {
	urlFlag := flag.String("u", "", "Single target URL")
	fileFlag := flag.String("f", "", "File containing target URLs")
	flag.Parse()

	urlChan := make(chan string, 100)

	go func() {
		defer close(urlChan)

		if *urlFlag != "" {
			urlChan <- *urlFlag
		}

		if *fileFlag != "" {
			readFile(*fileFlag, urlChan)
		}

		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			readStdin(urlChan)
		}
	}()

	// Using our new logger for output
	logger.Info("Cexss initialized. Loading targets...")
	
	for target := range urlChan {
		logger.Success("Target found: %s", target)
	}
}

func readFile(path string, out chan string) {
	file, err := os.Open(path)
	if err != nil {
		logger.Error("Could not open file: %v", err)
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

func readStdin(out chan string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			out <- line
		}
	}
}
