package main

import (
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <command> [args...]\n", os.Args[0])
		fmt.Println("Commands:")
		fmt.Println("  list <namespace> <kvv2-path>  List secret names")
		fmt.Println("  pull <namespace> <kvv2-path> [output-dir]  Pull all secrets recursively to files")
		fmt.Println("  push <namespace> <kvv2-path> [input-dir] [--dry-run]  Push secrets from YAML files to Vault")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "list":
		handleListCommand()
	case "pull":
		handlePullCommand()
	case "push":
		handlePushCommand()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}

func handleListCommand() {
	if len(os.Args) < 4 {
		fmt.Printf("Usage: %s list <namespace> <kvv2-path>\n", os.Args[0])
		fmt.Println("Example: go run . list my-namespace kv/metadata")
		os.Exit(1)
	}

	namespace := os.Args[2]
	kvPath := os.Args[3]

	client := getVaultClient(namespace)

	secrets, err := client.ListSecrets(kvPath)
	if err != nil {
		log.Fatalf("Failed to list secrets: %v", err)
	}

	if len(secrets) == 0 {
		fmt.Println("No secrets found at the specified path")
		return
	}

	fmt.Printf("Secrets at %s in namespace %s:\n", kvPath, namespace)
	for _, secret := range secrets {
		fmt.Printf("  - %s\n", secret)
	}
}

func handlePullCommand() {
	if len(os.Args) < 4 {
		fmt.Printf("Usage: %s pull <namespace> <kvv2-path> [output-dir]\n", os.Args[0])
		fmt.Println("Example: go run . pull my-namespace kv/metadata ./secrets")
		fmt.Println("If output-dir is not specified, defaults to './vault-secrets'")
		os.Exit(1)
	}

	namespace := os.Args[2]
	kvPath := os.Args[3]
	
	// Default output directory if not specified
	outputDir := "./vault-secrets"
	if len(os.Args) > 4 && !strings.HasPrefix(os.Args[4], "--") {
		outputDir = os.Args[4]
	}

	client := getVaultClient(namespace)

	fmt.Printf("Pulling all secrets recursively from %s in namespace %s to %s...\n", kvPath, namespace, outputDir)
	
	err := client.PullSecretsToFiles(kvPath, outputDir)
	if err != nil {
		log.Fatalf("Failed to pull secrets to files: %v", err)
	}

	fmt.Printf("\nCompleted! Secrets have been saved to %s as YAML files\n", outputDir)
}

func handlePushCommand() {
	if len(os.Args) < 4 {
		fmt.Printf("Usage: %s push <namespace> <kvv2-path> [input-dir] [--dry-run]\n", os.Args[0])
		fmt.Println("Example: go run . push my-namespace kv/metadata")
		fmt.Println("Example: go run . push my-namespace kv/metadata ./my-secrets")
		fmt.Println("If input-dir is not specified, defaults to './vault-secrets'")
		fmt.Println("Use --dry-run to see what would be changed without actually pushing")
		os.Exit(1)
	}

	namespace := os.Args[2]
	kvPath := os.Args[3]
	
	// Default input directory if not specified, and parse remaining args
	inputDir := "./vault-secrets"
	dryRun := false
	
	// Parse remaining arguments
	for i := 4; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--dry-run" {
			dryRun = true
		} else if !strings.HasPrefix(arg, "--") {
			inputDir = arg
		}
	}

	client := getVaultClient(namespace)

	if dryRun {
		fmt.Printf("DRY RUN: Showing what would be changed when pushing from %s to %s in namespace %s...\n", inputDir, kvPath, namespace)
	} else {
		fmt.Printf("Pushing secrets from %s to %s in namespace %s...\n", inputDir, kvPath, namespace)
	}

	err := client.PushSecretsFromFiles(inputDir, kvPath, dryRun)
	if err != nil {
		log.Fatalf("Failed to push secrets: %v", err)
	}

	if dryRun {
		fmt.Printf("\nDry run completed! Use without --dry-run to actually push changes.\n")
	} else {
		fmt.Printf("\nCompleted! Secrets have been pushed to Vault.\n")
	}
}

func getVaultClient(namespace string) *VaultClient {
	vaultAddr := os.Getenv("VAULT_ADDR")
	if vaultAddr == "" {
		log.Fatal("VAULT_ADDR environment variable is required")
	}

	vaultToken := os.Getenv("VAULT_TOKEN")
	if vaultToken == "" {
		log.Fatal("VAULT_TOKEN environment variable is required")
	}

	return NewVaultClient(vaultAddr, vaultToken, namespace)
}