// Package util contains helper functions
package util

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"regexp"
)

// GetEnvParameter returns the value of an environment variable
func GetEnvParameter(name string, mandatory bool) string {
	value := os.Getenv(name)
	if mandatory && value == "" {
		log.Printf("Mandatory environment parameter not set: %v", name)
	}
	return value
}

// CompileRePattern compiles a string containing a regular expression
func CompileRePattern(pattern string) *regexp.Regexp {
	if pattern == "" {
		pattern = "^$"
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		log.Printf("Could not compile regexp pattern: %v", pattern)
		return nil
	}
	return re
}

// ReadFile reads a file from the file system
func ReadFile(name string) ([]byte, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SaveFile saves a file to the file system
func SaveFile(name string, data []byte) error {
	return os.WriteFile(name, data, 0o644)
}

// MakeDirIfNotExists creates a directory if it does not exist yet
func MakeDirIfNotExists(directory string) error {
	if _, err := os.Stat(directory); errors.Is(err, os.ErrNotExist) {
		return os.Mkdir(directory, os.ModePerm)
	}
	return nil
}

// ReadLinesFromFile reads a file and returns a string slice containing the lines
func ReadLinesFromFile(name string) []string {
	file, err := os.Open(name)
	if err != nil {
		log.Printf("ERROR Could not open file %v : %v\n", name, err)
		return nil
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Printf("ERROR Could not close file %v : %v\n", name, err)
		}
	}(file)
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

// RandToken generates a random hex value.
func RandToken(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
