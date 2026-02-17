package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/rybkr/gitvista"
	"github.com/rybkr/gitvista/internal/gitcore"
	"github.com/rybkr/gitvista/internal/server"
)

const version = "1.0.0-dev"

func main() {
	// CLI flags
	repoPath := flag.String("repo", getEnv("GITVISTA_REPO", "."), "Path to git repository")
	port := flag.String("port", getEnv("GITVISTA_PORT", "8080"), "Port to listen on")
	host := flag.String("host", getEnv("GITVISTA_HOST", ""), "Host to bind to (empty = all interfaces)")
	showVersion := flag.Bool("version", false, "Show version and exit")
	showHelp := flag.Bool("help", false, "Show help and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("GitVista %s\n", version)
		os.Exit(0)
	}

	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	// Load repository
	repo, err := gitcore.NewRepository(*repoPath)
	if err != nil {
		log.Fatalf("Failed to load repository at %s: %v", *repoPath, err)
	}

	// Get embedded web filesystem
	webFS, err := gitvista.GetWebFS()
	if err != nil {
		log.Fatalf("Failed to load web assets: %v", err)
	}

	// Create and start server
	addr := fmt.Sprintf("%s:%s", *host, *port)
	serv := server.NewServer(repo, addr, webFS)

	log.Printf("Starting GitVista %s", version)
	log.Printf("Repository: %s", *repoPath)
	log.Printf("Listening on http://%s", addr)

	if err := serv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func printHelp() {
	fmt.Println("GitVista - Real-time Git repository visualization")
	fmt.Printf("Version: %s\n\n", version)
	fmt.Println("Usage:")
	fmt.Println("  vista [flags]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -repo string")
	fmt.Println("        Path to git repository (default: current directory)")
	fmt.Println("        Environment: GITVISTA_REPO")
	fmt.Println()
	fmt.Println("  -port string")
	fmt.Println("        Port to listen on (default: 8080)")
	fmt.Println("        Environment: GITVISTA_PORT")
	fmt.Println()
	fmt.Println("  -host string")
	fmt.Println("        Host to bind to (default: all interfaces)")
	fmt.Println("        Environment: GITVISTA_HOST")
	fmt.Println()
	fmt.Println("  -version")
	fmt.Println("        Show version and exit")
	fmt.Println()
	fmt.Println("  -help")
	fmt.Println("        Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  vista")
	fmt.Println("  vista -repo /path/to/repo")
	fmt.Println("  vista -port 3000")
	fmt.Println("  vista -host localhost -port 9090")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  GITVISTA_REPO   Default repository path")
	fmt.Println("  GITVISTA_PORT   Default port")
	fmt.Println("  GITVISTA_HOST   Default host")
}
