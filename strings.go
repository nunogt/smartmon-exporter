package main

import (
	"bufio"
	"strings"
)

// firstLine reads the first line from a string
func firstLine(text string) string {
	scanner := bufio.NewScanner(strings.NewReader(text))
	if !scanner.Scan() {
		panic("Unable to read first line")
	}
	return scanner.Text()
}
