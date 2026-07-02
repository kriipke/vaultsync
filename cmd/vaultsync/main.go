// Command vaultsync is the CLI entrypoint for syncing secrets between
// HashiCorp Vault and local YAML files. It wraps the vaultsync library API.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"vaultsync"
)

// Populated at build time via -ldflags "-X main.version=... -X main.buildTime=...".
var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses argv and dispatches to a command handler, returning the process
// exit code. It is separated from main so it can be exercised in tests.
func run(argv []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vaultsync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	kvEngine := fs.String("kv-engine", "kv", "Name of the KVv2 secret engine")

	if err := fs.Parse(argv); err != nil {
		return 2
	}

	rest := fs.Args()
	if len(rest) == 0 {
		printUsage(stdout)
		return 1
	}

	command := rest[0]
	cmdArgs := rest[1:]

	switch command {
	case "version", "--version":
		fmt.Fprintf(stdout, "vaultsync %s (built %s)\n", version, buildTime)
		return 0
	case "list":
		return cmdList(*kvEngine, cmdArgs, stdout, stderr)
	case "pull":
		return cmdPull(*kvEngine, cmdArgs, stdout, stderr)
	case "push":
		return cmdPush(*kvEngine, cmdArgs, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "Unknown command: %s\n", command)
		printUsage(stderr)
		return 1
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: vaultsync [--kv-engine=name] <command> [args...]")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  list <namespace> [path]                          List secret names")
	fmt.Fprintln(w, "  pull <namespace> [path] [output-dir]             Pull secrets recursively to files")
	fmt.Fprintln(w, "  push <namespace> [path] [input-dir] [--dry-run]  Push secrets from YAML files to Vault")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --kv-engine string   Name of the KVv2 secret engine (default \"kv\")")
}

// looksLikePath reports whether an argument should be treated as a filesystem
// path (output/input directory) rather than a Vault sub-path.
func looksLikePath(arg string) bool {
	return strings.HasPrefix(arg, "./") ||
		strings.HasPrefix(arg, "../") ||
		strings.HasPrefix(arg, "/") ||
		strings.HasPrefix(arg, "~")
}

// pathDesc renders a human-readable description of the targeted KV path.
func pathDesc(kvEngine, subPath string) string {
	if subPath == "" {
		return kvEngine
	}
	return kvEngine + "/" + subPath
}

func newClient(namespace string, stdout, stderr io.Writer) (*vaultsync.VaultClient, error) {
	client, err := vaultsync.NewVaultClientFromEnv(namespace)
	if err != nil {
		return nil, err
	}
	client.Output = stdout
	client.ErrOutput = stderr
	return client, nil
}

func cmdList(kvEngine string, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "Usage: vaultsync [--kv-engine=name] list <namespace> [path]")
		return 1
	}

	namespace := args[0]
	subPath := ""
	if len(args) > 1 {
		subPath = args[1]
	}

	client, err := newClient(namespace, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	ref := vaultsync.NewSecretRef(kvEngine, subPath)
	secrets, err := client.ListSecretsAt(ref)
	if err != nil {
		fmt.Fprintf(stderr, "Failed to list secrets: %v\n", err)
		return 1
	}

	if len(secrets) == 0 {
		fmt.Fprintln(stdout, "No secrets found at the specified path")
		return 0
	}

	fmt.Fprintf(stdout, "Secrets at %s in namespace %s:\n", pathDesc(kvEngine, subPath), namespace)
	for _, secret := range secrets {
		fmt.Fprintf(stdout, "  - %s\n", secret)
	}
	return 0
}

// pullArgs holds the parsed positional arguments for the pull command.
type pullArgs struct {
	namespace string
	subPath   string
	outputDir string
}

func parsePullArgs(args []string) (pullArgs, error) {
	if len(args) < 1 {
		return pullArgs{}, fmt.Errorf("namespace is required")
	}

	parsed := pullArgs{namespace: args[0]}
	rest := args[1:]
	if len(rest) > 0 {
		if looksLikePath(rest[0]) {
			parsed.outputDir = rest[0]
		} else {
			parsed.subPath = rest[0]
			if len(rest) > 1 {
				parsed.outputDir = rest[1]
			}
		}
	}

	if parsed.outputDir == "" {
		parsed.outputDir = "./secrets"
	}
	return parsed, nil
}

func cmdPull(kvEngine string, args []string, stdout, stderr io.Writer) int {
	parsed, err := parsePullArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, "Usage: vaultsync [--kv-engine=name] pull <namespace> [path] [output-dir]")
		return 1
	}

	client, err := newClient(parsed.namespace, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	ref := vaultsync.NewSecretRef(kvEngine, parsed.subPath)
	fmt.Fprintf(stdout, "Pulling secrets from %s in namespace %s to %s...\n",
		pathDesc(kvEngine, parsed.subPath), parsed.namespace, parsed.outputDir)

	if err := client.PullSecretsToFilesAt(ref, parsed.outputDir); err != nil {
		fmt.Fprintf(stderr, "Failed to pull secrets: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Completed! Secrets saved to %s as YAML files\n", parsed.outputDir)
	return 0
}

// pushArgs holds the parsed positional arguments and flags for the push command.
type pushArgs struct {
	namespace string
	subPath   string
	inputDir  string
	dryRun    bool
}

func parsePushArgs(args []string) (pushArgs, error) {
	var parsed pushArgs
	var positional []string

	for _, arg := range args {
		if arg == "--dry-run" {
			parsed.dryRun = true
			continue
		}
		positional = append(positional, arg)
	}

	if len(positional) < 1 {
		return pushArgs{}, fmt.Errorf("namespace is required")
	}

	parsed.namespace = positional[0]
	rest := positional[1:]
	if len(rest) > 0 {
		if looksLikePath(rest[0]) {
			parsed.inputDir = rest[0]
		} else {
			parsed.subPath = rest[0]
			if len(rest) > 1 {
				parsed.inputDir = rest[1]
			}
		}
	}

	if parsed.inputDir == "" {
		parsed.inputDir = "./secrets"
	}
	return parsed, nil
}

func cmdPush(kvEngine string, args []string, stdout, stderr io.Writer) int {
	parsed, err := parsePushArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, "Usage: vaultsync [--kv-engine=name] push <namespace> [path] [input-dir] [--dry-run]")
		return 1
	}

	client, err := newClient(parsed.namespace, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}

	ref := vaultsync.NewSecretRef(kvEngine, parsed.subPath)
	if parsed.dryRun {
		fmt.Fprintf(stdout, "DRY RUN: showing changes for push from %s to %s in namespace %s...\n",
			parsed.inputDir, pathDesc(kvEngine, parsed.subPath), parsed.namespace)
	} else {
		fmt.Fprintf(stdout, "Pushing secrets from %s to %s in namespace %s...\n",
			parsed.inputDir, pathDesc(kvEngine, parsed.subPath), parsed.namespace)
	}

	if err := client.PushSecretsFromFilesAt(parsed.inputDir, ref, parsed.dryRun); err != nil {
		fmt.Fprintf(stderr, "Push operation failed: %v\n", err)
		return 1
	}

	if parsed.dryRun {
		fmt.Fprintln(stdout, "Dry run completed! Use without --dry-run to actually push changes.")
	} else {
		fmt.Fprintln(stdout, "Completed! Secrets have been pushed to Vault.")
	}
	return 0
}
