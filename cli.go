package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

var kvEngine = flag.String("kv-engine", "kv", "Name of the KVv2 secret engine")

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s [--kv-engine=name] <command> [args...]\n", os.Args[0])
		fmt.Println("Commands:")
		fmt.Println("  list <namespace> [path]  List secret names")
		fmt.Println("  pull <namespace> [path] [output-dir]  Pull all secrets recursively to files")
		fmt.Println("  push <namespace> [path] [input-dir] [--dry-run]  Push secrets from YAML files to Vault")
		fmt.Println("")
		fmt.Println("Flags:")
		fmt.Println("  --kv-engine string   Name of the KVv2 secret engine (default \"kv\")")
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  vaultsync list my-namespace")
		fmt.Println("  vaultsync list my-namespace app")
		fmt.Println("  vaultsync --kv-engine=secrets list my-namespace app")
		os.Exit(1)
	}

	// Parse flags but preserve command and args
	args := os.Args[1:]
	var command string
	var commandArgs []string
	
	// Find first non-flag argument as command
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			command = arg
			commandArgs = args[i+1:]
			// Parse flags before the command
			flag.CommandLine.Parse(args[:i])
			break
		}
	}
	
	if command == "" {
		fmt.Println("Error: No command specified")
		os.Exit(1)
	}

	switch command {
	case "list":
		handleListCommand(commandArgs)
	case "pull":
		handlePullCommand(commandArgs)
	case "push":
		handlePushCommand(commandArgs)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}
}

func handleListCommand(args []string) {
	if len(args) < 1 {
		fmt.Printf("Usage: vaultsync [--kv-engine=name] list <namespace> [path]\n")
		fmt.Println("Example: vaultsync list my-namespace")
		fmt.Println("Example: vaultsync list my-namespace app")
		os.Exit(1)
	}

	namespace := args[0]
	var subPath string
	if len(args) > 1 {
		subPath = args[1]
	}

	// Construct the full metadata path
	var metadataPath string
	if subPath == "" {
		metadataPath = *kvEngine + "/metadata"
	} else {
		metadataPath = *kvEngine + "/metadata/" + subPath
	}

	client := getVaultClient(namespace)

	secrets, err := client.ListSecrets(metadataPath)
	if err != nil {
		log.Fatalf("Failed to list secrets: %v", err)
	}

	if len(secrets) == 0 {
		fmt.Println("No secrets found at the specified path")
		return
	}

	pathDesc := *kvEngine
	if subPath != "" {
		pathDesc += "/" + subPath
	}
	fmt.Printf("Secrets at %s in namespace %s:\n", pathDesc, namespace)
	for _, secret := range secrets {
		fmt.Printf("  - %s\n", secret)
	}
}

func handlePullCommand(args []string) {
	if len(args) < 1 {
		fmt.Printf("Usage: vaultsync [--kv-engine=name] pull <namespace> [path] [output-dir]\n")
		fmt.Println("Example: vaultsync pull my-namespace")
		fmt.Println("Example: vaultsync pull my-namespace app ./secrets")
		fmt.Println("If output-dir is not specified, defaults to './vault-secrets'")
		os.Exit(1)
	}

	namespace := args[0]
	var subPath string
	var outputDir string
	
	// Parse remaining args
	if len(args) > 1 {
		if strings.HasPrefix(args[1], "./") || strings.HasPrefix(args[1], "/") || strings.HasPrefix(args[1], "~") {
			// First arg looks like a directory path
			outputDir = args[1]
		} else {
			// First arg is subPath
			subPath = args[1]
			if len(args) > 2 {
				outputDir = args[2]
			}
		}
	}
	
	// Default output directory if not specified
	if outputDir == "" {
		outputDir = "./vault-secrets"
	}

	// Construct the full metadata path
	var metadataPath string
	if subPath == "" {
		metadataPath = *kvEngine + "/metadata"
	} else {
		metadataPath = *kvEngine + "/metadata/" + subPath
	}

	client := getVaultClient(namespace)

	pathDesc := *kvEngine
	if subPath != "" {
		pathDesc += "/" + subPath
	}
	fmt.Printf("Pulling all secrets recursively from %s in namespace %s to %s...\n", pathDesc, namespace, outputDir)
	
	err := client.PullSecretsToFiles(metadataPath, outputDir)
	if err != nil {
		log.Fatalf("Failed to pull secrets to files: %v", err)
	}

	fmt.Printf("\nCompleted! Secrets have been saved to %s as YAML files\n", outputDir)
}

func handlePushCommand(args []string) {
	if len(args) < 1 {
		fmt.Printf("Usage: vaultsync [--kv-engine=name] push <namespace> [path] [input-dir] [--dry-run]\n")
		fmt.Println("Example: vaultsync push my-namespace --dry-run")
		fmt.Println("Example: vaultsync push my-namespace app ./my-secrets")
		fmt.Println("If input-dir is not specified, defaults to './vault-secrets'")
		fmt.Println("Use --dry-run to see what would be changed without actually pushing")
		os.Exit(1)
	}

	namespace := args[0]
	var subPath string
	var inputDir string
	dryRun := false
	
	// Parse remaining arguments
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--dry-run" {
			dryRun = true
		} else if strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "~") {
			// Looks like a directory path
			inputDir = arg
		} else if !strings.HasPrefix(arg, "--") {
			// Must be subPath
			subPath = arg
		}
	}
	
	// Default input directory if not specified
	if inputDir == "" {
		inputDir = "./vault-secrets"
	}

	// Construct the full metadata path
	var metadataPath string
	if subPath == "" {
		metadataPath = *kvEngine + "/metadata"
	} else {
		metadataPath = *kvEngine + "/metadata/" + subPath
	}

	client := getVaultClient(namespace)

	pathDesc := *kvEngine
	if subPath != "" {
		pathDesc += "/" + subPath
	}
	
	if dryRun {
		fmt.Printf("DRY RUN: Showing what would be changed when pushing from %s to %s in namespace %s...\n", inputDir, pathDesc, namespace)
	} else {
		fmt.Printf("Pushing secrets from %s to %s in namespace %s...\n", inputDir, pathDesc, namespace)
	}

	fmt.Printf("[DEBUG] Starting push operation with client config:\\n")
	fmt.Printf("[DEBUG]   Vault Address: %s\\n", client.Address)
	fmt.Printf("[DEBUG]   Namespace: %s\\n", client.Namespace)
	fmt.Printf("[DEBUG]   Token length: %d characters\\n", len(client.Token))
	
	// Test connectivity before attempting the push
	if err := client.TestConnectivity(); err != nil {
		fmt.Printf("[ERROR] Vault connectivity test failed: %v\\n", err)
		log.Fatalf("Cannot connect to Vault: %v", err)
	}
	
	err := client.PushSecretsFromFiles(inputDir, metadataPath, dryRun)
	if err != nil {
		fmt.Printf("[ERROR] Push operation failed: %v\\n", err)
		fmt.Printf("[DEBUG] Common issues to check:\\n")
		fmt.Printf("[DEBUG]   - Verify VAULT_ADDR is correct and accessible\\n")
		fmt.Printf("[DEBUG]   - Verify VAULT_TOKEN has sufficient permissions\\n")
		fmt.Printf("[DEBUG]   - Verify namespace '%s' exists and is accessible\\n", namespace)
		fmt.Printf("[DEBUG]   - Check if KV engine '%s' is mounted and accessible\\n", *kvEngine)
		fmt.Printf("[DEBUG]   - Ensure input directory '%s' contains valid YAML files\\n", inputDir)
		log.Fatalf("Push operation failed: %v", err)
	}

	if dryRun {
		fmt.Printf("\nDry run completed! Use without --dry-run to actually push changes.\n")
	} else {
		fmt.Printf("\nCompleted! Secrets have been pushed to Vault.\n")
	}
}

func getVaultClient(namespace string) *VaultClient {
	fmt.Printf("[DEBUG] Creating Vault client...\\n")
	
	vaultAddr := os.Getenv("VAULT_ADDR")
	if vaultAddr == "" {
		fmt.Printf("[ERROR] VAULT_ADDR environment variable is not set\\n")
		fmt.Printf("[DEBUG] Please set VAULT_ADDR to your Vault server URL (e.g., https://vault.example.com:8200)\\n")
		log.Fatal("VAULT_ADDR environment variable is required")
	}
	fmt.Printf("[DEBUG] Using VAULT_ADDR: %s\\n", vaultAddr)

	vaultToken := os.Getenv("VAULT_TOKEN")
	if vaultToken == "" {
		fmt.Printf("[ERROR] VAULT_TOKEN environment variable is not set\\n")
		fmt.Printf("[DEBUG] Please set VAULT_TOKEN to a valid Vault authentication token\\n")
		log.Fatal("VAULT_TOKEN environment variable is required")
	}
	fmt.Printf("[DEBUG] VAULT_TOKEN is set (%d characters)\\n", len(vaultToken))

	if namespace == "" {
		fmt.Printf("[ERROR] Namespace cannot be empty\\n")
		log.Fatal("Namespace is required")
	}
	fmt.Printf("[DEBUG] Using namespace: %s\\n", namespace)

	client := NewVaultClient(vaultAddr, vaultToken, namespace)
	fmt.Printf("[DEBUG] Vault client created successfully\\n")
	
	return client
}