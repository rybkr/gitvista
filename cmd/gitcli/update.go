package main

import (
	"fmt"
	"os"

	"github.com/rybkr/gitvista/internal/selfupdate"
)

const ghRepo = "rybkr/gitvista"

func runUpdate(args []string) int {
	checkOnly := false
	for _, a := range args {
		if a == "--check" || a == "-check" {
			checkOnly = true
		}
	}

	fmt.Printf("Current version: %s\n", version)

	latest, err := selfupdate.CheckLatest(ghRepo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking for updates: %v\n", err)
		return 1
	}
	fmt.Printf("Latest version:  %s\n", latest)

	if !selfupdate.NeedsUpdate(version, latest) {
		if version == "dev" {
			fmt.Println("Development build — skipping update.")
		} else {
			fmt.Println("Already up to date.")
		}
		return 0
	}

	if checkOnly {
		fmt.Printf("Update available: %s → %s\n", version, latest)
		fmt.Println("Run 'gitvista-cli update' to install it.")
		return 0
	}

	fmt.Printf("Updating to %s...\n", latest)
	if err := selfupdate.Update(ghRepo, "gitvista-cli", latest); err != nil {
		fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully updated to %s\n", latest)
	return 0
}
