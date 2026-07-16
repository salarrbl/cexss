package input

import (
	"bufio"
	"os"
	"strings"
)

func Targets(single, filePath string) []string {
	seen := map[string]struct{}{}
	var targets []string

	add := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			targets = append(targets, t)
		}
	}

	if single != "" {
		add(single)
	}

	if filePath != "" {
		f, err := os.Open(filePath)
		if err == nil {
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				add(scanner.Text())
			}
			f.Close()
		}
	}

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			add(scanner.Text())
		}
	}

	return targets
}

func ReadLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}
