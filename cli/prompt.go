package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// promptConfirmation prompts the user for confirmation
func promptConfirmation(message string) (bool, error) {
	fmt.Printf("%s (y/N): ", message)
	var response string
	if _, err := fmt.Scanln(&response); err != nil {
		return false, fmt.Errorf("failed to read confirmation: %w", err)
	}
	return strings.ToLower(strings.TrimSpace(response)) == "y", nil
}

func promptForInput(prompt string) (string, error) {
	fmt.Printf("%s: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}
	return strings.TrimSpace(input), nil
}

// promptForPassword prompts the user for password input (hidden)
func promptForPassword(prompt string) (string, error) {
	fmt.Printf("%s: ", prompt)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println() // Print newline after password input
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}
	return string(bytePassword), nil
}
