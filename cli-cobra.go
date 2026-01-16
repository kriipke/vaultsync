package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var (
	dryRun bool
	rootCmd = &cobra.Command{
		Use:   "vault-sync",
		Short: "A CLI tool to sync secrets between Vault and local YAML files",
	}

	listCmd = &cobra.Command{
		Use:   "list <namespace> <kvv2-path>",
		Short: "List secret names",
		Args:  cobra.ExactArgs(2),
		Run:   runListCommand,
	}

	pullCmd = &cobra.Command{
		Use:   "pull <namespace> <kvv2-path> [output-dir]",
		Short: "Pull all secrets recursively to files",
		Args:  cobra.RangeArgs(2, 3),
		Run:   runPullCommand,
	}

	pushCmd = &cobra.Command{
		Use:   "push <namespace> <kvv2-path> <input-dir>",
		Short: "Push secrets from YAML files to Vault",
		Args:  cobra.ExactArgs(3),
		Run:   runPushCommand,
	}
)

func init() {
	pushCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be changed without actually pushing")
	rootCmd.AddCommand(listCmd, pullCmd, pushCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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

func runListCommand(cmd *cobra.Command, args []string) {
	namespace := args[0]
	kvPath := args[1]

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

func runPullCommand(cmd *cobra.Command, args []string) {
	namespace := args[0]
	kvPath := args[1]
	
	// Default output directory if not specified
	outputDir := "./vault-secrets"
	if len(args) > 2 {
		outputDir = args[2]
	}

	client := getVaultClient(namespace)

	fmt.Printf("Pulling all secrets recursively from %s in namespace %s to %s...\n", kvPath, namespace, outputDir)
	
	err := client.PullSecretsToFiles(kvPath, outputDir)
	if err != nil {
		log.Fatalf("Failed to pull secrets to files: %v", err)
	}

	fmt.Printf("\nCompleted! Secrets have been saved to %s as YAML files\n", outputDir)
}

func runPushCommand(cmd *cobra.Command, args []string) {
	namespace := args[0]
	kvPath := args[1]
	inputDir := args[2]

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