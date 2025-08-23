package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(os.Stdout, "$ ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading input:", err)
		}
		if input == "\n" {
			continue
		}
		input = strings.TrimSpace(input)
		parts := splitArguments(input)
		comm := parts[0]
		args := parts[1:]
		command, err := commandParser(comm, args)
		if err != nil {
			fmt.Printf("%v\n", err)
			continue
		}
		err = command.run()
		if err != nil {
			// Only print error if it's a built-in or redirection failed
			// External commands like `cat` already print their own errors
			if _, ok := command.(*externalComm); !ok {
				fmt.Printf("Error executing command: %v\n", err)
			}
		}
	}
}
func isShellBuiltin(command string) bool {
	builtins := []string{"echo", "exit", "type", "pwd", "cd"}
	for _, b := range builtins {
		if b == command {
			return true
		}
	}
	return false
}

type comm interface {
	run() error
}
type exitComm struct {
	statusCode int
}
type echoComm struct {
	args       []string
	stdoutFile string
	stderrFile string
}
type typeComm struct {
	commName string
}
type externalComm struct {
	path       string
	args       []string
	stdoutFile string
	stderrFile string
}
type pwdComm struct{}
type cdComm struct {
	path string
	args []string
}

func (c *cdComm) run() error {
	if c.path == "" || c.path == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cd: unable to get home directory: %v", err)
		}
		c.path = homeDir
	}
	err := os.Chdir(c.path)
	if err != nil {
		fmt.Printf("cd: %s: No such file or directory\n", c.args[0])
		return nil
	}
	return nil
}

func (p *pwdComm) run() error {
	currentDir, err := os.Getwd()
	if err != nil {
		return err
	}
	fmt.Println(currentDir)
	return nil
}
func (e *exitComm) run() error {
	os.Exit(e.statusCode)
	return nil
}
func (e *echoComm) run() error {
	output := strings.Join(e.args, " ")
	if e.stdoutFile != "" {
		file, err := os.Create(e.stdoutFile)
		if err != nil {
			return fmt.Errorf("error opening redirection file: %v", err)
		}
		defer file.Close()
		_, err = fmt.Fprintln(file, output)
		return err
	}
	fmt.Println(output)
	return nil
}
func (e *externalComm) run() error {
	cmd := exec.Command(e.path, e.args...)
	cmd.Args[0] = filepath.Base(e.path)
	if e.stdoutFile != "" {
		file, err := os.Create(e.stdoutFile)
		if err != nil {
			return fmt.Errorf("error opening redirection file: %v", err)
		}
		defer file.Close()
		cmd.Stdout = file
	} else {
		cmd.Stdout = os.Stdout
	}
	if e.stderrFile != "" {
		file, err := os.Create(e.stderrFile)
		if err != nil {
			return fmt.Errorf("error opening redirection file: %v", err)
		}
		defer file.Close()
		cmd.Stderr = file
	} else {
		cmd.Stderr = os.Stderr
	}
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
func (t *typeComm) run() error {
	if isShellBuiltin(t.commName) {
		fmt.Printf("%s is a shell builtin\n", t.commName)
		return nil
	}
	pathEnv := os.Getenv("PATH")
	paths := strings.Split(pathEnv, ":")
	for _, dir := range paths {
		fullPath := filepath.Join(dir, t.commName)
		if fileInfo, err := os.Stat(fullPath); err == nil && !fileInfo.IsDir() && fileInfo.Mode()&0111 != 0 {
			fmt.Println(fullPath)
			return nil
		}
	}
	fmt.Printf("%s: not found\n", t.commName)
	return nil
}
func splitArguments(input string) []string {
	var args []string
	var curr strings.Builder
	inSingleQuotes := false //inside or outside quotes
	inDoubleQuotes := false //inside or outside quotes
	withEscape := false     //if escape character is used
	for _, r := range input {
		switch {
		case withEscape:
			// Only $, ", and \ are escapable inside double quotes
			if inDoubleQuotes {
				if r == '$' || r == '"' || r == '\\' {
					curr.WriteRune(r)
				} else {
					curr.WriteRune('\\') // If we encounter a character that is not escapable, we write the escape character and the character itself
					curr.WriteRune(r)
				}
			} else {
				curr.WriteRune(r)
			}
			withEscape = false
		case r == '\\' && !inSingleQuotes: // escape character '\'
			// If we encounter a backslash inside or outside of double quotes, we just skip it
			withEscape = true //set escape mode
		case r == '\'' && !inDoubleQuotes: // double or single quote
			inSingleQuotes = !inSingleQuotes
		case r == '"' && !inSingleQuotes: // double or single quote
			inDoubleQuotes = !inDoubleQuotes
		case r == ' ' && !inSingleQuotes && !inDoubleQuotes: // space character
			if curr.Len() > 0 {
				args = append(args, curr.String()) //.string() converts the builder to a string
				curr.Reset()                       // reset the builder for next arg
			}
		default:
			curr.WriteRune(r)
		}
	}
	if curr.Len() > 0 {
		args = append(args, curr.String())
	}
	return args
}
func extractRedirection(args []string) ([]string, string, string) {
	var cleanedArgs []string
	var redirectFile string
	var stderrFile string
	skip := false

	for i := 0; i < len(args); i++ {
		if skip {
			skip = false
			continue
		}
		if args[i] == ">" || args[i] == "1>" {
			if i+1 < len(args) {
				redirectFile = args[i+1]
				skip = true // skip next arg (filename)
			}
		} else if args[i] == "2>" {
			if i+1 < len(args) {
				stderrFile = args[i+1]
				skip = true // skip next arg (filename)
			}
		}else {
			cleanedArgs = append(cleanedArgs, args[i])
		}
		
	}
	return cleanedArgs, redirectFile, stderrFile
}
func commandParser(command string, args []string) (comm, error) {
	cleanArgs, stdoutFile, stderrFile := extractRedirection(args)
	if command == "exit" {
		if len(cleanArgs) == 0 {
			return &exitComm{0}, nil
		}
		statusCode, _ := strconv.Atoi(cleanArgs[0])
		return &exitComm{statusCode}, nil
	}
	if command == "echo" {
		if len(cleanArgs) == 0 {
			return nil, fmt.Errorf("echo: no arguments provided")
		}
		return &echoComm{cleanArgs, stdoutFile,stderrFile}, nil
	}
	if command == "type" {
		if len(cleanArgs) == 0 {
			return nil, fmt.Errorf("type: no arguments provided")
		}
		return &typeComm{cleanArgs[0]}, nil
	}
	if command == "pwd" {
		return &pwdComm{}, nil
	}
	if command == "cd" {
		path := ""
		if len(args) > 0 {
			path = args[0]
		}
		return &cdComm{path, cleanArgs}, nil
	}
	pathEnv := os.Getenv("PATH")
	paths := strings.Split(pathEnv, ":")
	for _, dir := range paths {
		fullPath := filepath.Join(dir, command)
		if fileInfo, err := os.Stat(fullPath); err == nil && !fileInfo.IsDir() && fileInfo.Mode()&0111 != 0 {
			return &externalComm{path: fullPath, args: cleanArgs, stdoutFile: stdoutFile, stderrFile: stderrFile}, nil
		}
	}
	return nil, fmt.Errorf("%s: command not found", command)
}
