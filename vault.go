package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type VaultClient struct {
	Address   string
	Token     string
	Namespace string
	client    *http.Client
}

type VaultListResponse struct {
	Data struct {
		Keys []string `json:"keys"`
	} `json:"data"`
}

type VaultSecretResponse struct {
	Data struct {
		Data     map[string]interface{} `json:"data"`
		Metadata struct {
			Version int `json:"version"`
		} `json:"metadata"`
	} `json:"data"`
}

func NewVaultClient(address, token, namespace string) *VaultClient {
	return &VaultClient{
		Address:   address,
		Token:     token,
		Namespace: namespace,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (v *VaultClient) ListSecrets(kvPath string) ([]string, error) {
	url := fmt.Sprintf("%s/v1/%s?list=true", v.Address, kvPath)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Vault-Token", v.Token)
	req.Header.Set("X-Vault-Namespace", v.Namespace)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var vaultResp VaultListResponse
	if err := json.Unmarshal(body, &vaultResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return vaultResp.Data.Keys, nil
}

func (v *VaultClient) GetSecret(secretPath string) (map[string]interface{}, error) {
	// Convert metadata path to data path for KVv2
	// Handle different KV engine names
	dataPath := secretPath
	parts := strings.Split(secretPath, "/")
	if len(parts) >= 2 && parts[1] == "metadata" {
		// Replace "/metadata/" with "/data/" in the path
		dataPath = parts[0] + "/data/" + strings.Join(parts[2:], "/")
	}

	fmt.Printf("Debug: Getting secret from path: %s (converted to: %s)\n", secretPath, dataPath)
	url := fmt.Sprintf("%s/v1/%s", v.Address, dataPath)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Vault-Token", v.Token)
	req.Header.Set("X-Vault-Namespace", v.Namespace)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var vaultResp VaultSecretResponse
	if err := json.Unmarshal(body, &vaultResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return vaultResp.Data.Data, nil
}

func (v *VaultClient) PullSecretsRecursively(basePath string) (map[string]map[string]interface{}, error) {
	secrets := make(map[string]map[string]interface{})
	
	return v.pullSecretsRecursivelyHelper(basePath, secrets)
}

func (v *VaultClient) pullSecretsRecursivelyHelper(currentPath string, secrets map[string]map[string]interface{}) (map[string]map[string]interface{}, error) {
	keys, err := v.ListSecrets(currentPath)
	if err != nil {
		return secrets, fmt.Errorf("failed to list secrets at %s: %w", currentPath, err)
	}

	for _, key := range keys {
		fullPath := currentPath + "/" + key
		
		// If key ends with /, it's a folder - recurse into it
		if key[len(key)-1] == '/' {
			folderPath := currentPath + "/" + key[:len(key)-1] + "/metadata"
			secrets, err = v.pullSecretsRecursivelyHelper(folderPath, secrets)
			if err != nil {
				return secrets, err
			}
		} else {
			// It's a secret - fetch its data
			// Build correct path for secret data
			secretPath := currentPath + "/" + key
			// Convert from metadata path to data path
			if len(currentPath) >= 11 && currentPath[:11] == "kv/metadata" {
				secretPath = "kv/data" + currentPath[11:] + "/" + key
			}
			
			secretData, err := v.GetSecret(secretPath)
			if err != nil {
				fmt.Printf("Warning: Failed to get secret %s: %v\n", fullPath, err)
				continue
			}
			secrets[fullPath] = secretData
		}
	}

	return secrets, nil
}

func (v *VaultClient) PullSecretsToFiles(basePath, outputDir string) error {
	secrets, err := v.PullSecretsRecursively(basePath)
	if err != nil {
		return fmt.Errorf("failed to pull secrets: %w", err)
	}

	for secretPath, secretData := range secrets {
		if err := v.writeSecretToFile(secretPath, secretData, basePath, outputDir); err != nil {
			fmt.Printf("Warning: Failed to write secret %s: %v\n", secretPath, err)
		}
	}

	return nil
}

func (v *VaultClient) writeSecretToFile(secretPath string, secretData map[string]interface{}, metadataPath, outputDir string) error {
	// metadataPath is like "kv/metadata" or "kv/metadata/app"
	// secretPath is like "kv/metadata/app/db" or "kv/metadata/shared/config"
	
	// Extract the relative path from the secret path
	relativePath := strings.TrimPrefix(secretPath, metadataPath)
	relativePath = strings.TrimPrefix(relativePath, "/") // Remove leading slash if present
	
	if relativePath == "" {
		// Handle edge case where secret name would be empty
		return fmt.Errorf("cannot determine file name for secret %s", secretPath)
	}
	
	// Extract subpath from metadataPath to determine target directory
	parts := strings.Split(metadataPath, "/")
	var targetDir string
	if len(parts) > 2 {
		// metadataPath has subpath like "kv/metadata/app"
		subPath := strings.Join(parts[2:], "/")
		targetDir = filepath.Join(outputDir, subPath)
	} else {
		// metadataPath is just "kv/metadata"
		targetDir = outputDir
	}
	
	// Create file path with .yaml extension
	filePath := filepath.Join(targetDir, relativePath+".yaml")
	
	// Create directory structure
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Convert to YAML
	yamlData, err := yaml.Marshal(secretData)
	if err != nil {
		return fmt.Errorf("failed to convert to YAML: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filePath, yamlData, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	fmt.Printf("Written: %s\n", filePath)
	return nil
}

func (v *VaultClient) PutSecret(secretPath string, secretData map[string]interface{}) error {
	// Convert metadata path to data path for KVv2
	// Handle different KV engine names
	dataPath := secretPath
	parts := strings.Split(secretPath, "/")
	if len(parts) >= 2 && parts[1] == "metadata" {
		// Replace "/metadata/" with "/data/" in the path
		dataPath = parts[0] + "/data/" + strings.Join(parts[2:], "/")
	}

	url := fmt.Sprintf("%s/v1/%s", v.Address, dataPath)

	// KVv2 requires wrapping data in a "data" field
	payload := map[string]interface{}{
		"data": secretData,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Vault-Token", v.Token)
	req.Header.Set("X-Vault-Namespace", v.Namespace)
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (v *VaultClient) PushSecretsFromFiles(inputDir, metadataPath string, dryRun bool) error {
	// metadataPath is like "kv/metadata" or "kv/metadata/app"
	// We need to derive the base directory for file discovery
	var baseDir string
	
	// Extract the KV engine name and subpath
	parts := strings.Split(metadataPath, "/")
	if len(parts) < 2 || parts[1] != "metadata" {
		return fmt.Errorf("invalid metadata path: %s", metadataPath)
	}
	
	kvEngine := parts[0]
	var subPath string
	if len(parts) > 2 {
		subPath = strings.Join(parts[2:], "/")
		baseDir = filepath.Join(inputDir, subPath)
	} else {
		baseDir = inputDir
	}

	// Check if base directory exists
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return fmt.Errorf("directory %s does not exist (derived from vault path %s)", baseDir, metadataPath)
	}

	return filepath.Walk(baseDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-YAML files
		if info.IsDir() || !strings.HasSuffix(filePath, ".yaml") {
			return nil
		}

		// Read YAML file
		yamlData, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		// Parse YAML
		var secretData map[string]interface{}
		if err := yaml.Unmarshal(yamlData, &secretData); err != nil {
			return fmt.Errorf("failed to parse YAML in %s: %w", filePath, err)
		}

		// Convert file path back to vault path
		relativePath, err := filepath.Rel(baseDir, filePath)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Remove .yaml extension and convert to vault path
		secretPath := strings.TrimSuffix(relativePath, ".yaml")
		secretPath = strings.ReplaceAll(secretPath, string(filepath.Separator), "/")
		
		// Construct full vault metadata path
		var vaultPath string
		if subPath != "" {
			vaultPath = kvEngine + "/metadata/" + subPath + "/" + secretPath
		} else {
			vaultPath = kvEngine + "/metadata/" + secretPath
		}

		if dryRun {
			return v.showDryRunDiff(vaultPath, secretData)
		} else {
			fmt.Printf("Pushing: %s\n", vaultPath)
			return v.PutSecret(vaultPath, secretData)
		}
	})
}

func (v *VaultClient) showDryRunDiff(vaultPath string, newData map[string]interface{}) error {
	// Try to get existing secret
	existingData, err := v.GetSecret(vaultPath)
	
	var existingYaml []byte
	if err != nil {
		// Secret doesn't exist, use empty content
		existingYaml = []byte{}
	} else {
		existingYaml, _ = yaml.Marshal(existingData)
	}
	
	newYaml, _ := yaml.Marshal(newData)
	
	// Generate unified diff
	var diffOutput string
	
	if err != nil {
		// New file case
		var newFileDiff bytes.Buffer
		newFileDiff.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", vaultPath, vaultPath))
		newFileDiff.WriteString("new file mode 100644\n")
		newFileDiff.WriteString(fmt.Sprintf("index 0000000..%s\n", generateShortHash(string(newYaml))))
		newFileDiff.WriteString("--- /dev/null\n")
		newFileDiff.WriteString(fmt.Sprintf("+++ b/%s\n", vaultPath))
		for _, line := range strings.Split(string(newYaml), "\n") {
			if line != "" || strings.HasSuffix(string(newYaml), "\n") {
				newFileDiff.WriteString(fmt.Sprintf("+%s\n", line))
			}
		}
		diffOutput = newFileDiff.String()
	} else {
		diffOutput = generateUnifiedDiff(string(existingYaml), string(newYaml), vaultPath)
	}
	
	// Only output if there are changes
	if diffOutput != "" {
		outputDiff(diffOutput)
	}
	
	return nil
}

func generateUnifiedDiff(existing, new, filename string) string {
	if existing == new {
		return "" // No changes
	}
	
	existingLines := strings.Split(existing, "\n")
	newLines := strings.Split(new, "\n")
	
	var diff bytes.Buffer
	
	// Header
	diff.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", filename, filename))
	diff.WriteString(fmt.Sprintf("index %s..%s 100644\n", generateShortHash(existing), generateShortHash(new)))
	diff.WriteString(fmt.Sprintf("--- a/%s\n", filename))
	diff.WriteString(fmt.Sprintf("+++ b/%s\n", filename))
	
	// Simple line-by-line diff (basic implementation)
	maxLines := len(existingLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}
	
	inHunk := false
	hunkStart := 0
	hunkLines := []string{}
	
	for i := 0; i < maxLines; i++ {
		existingLine := ""
		newLine := ""
		
		if i < len(existingLines) {
			existingLine = existingLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}
		
		if existingLine != newLine {
			if !inHunk {
				// Start new hunk
				inHunk = true
				hunkStart = max(0, i-3) // 3 lines of context
				hunkLines = []string{}
				
				// Add context before the change
				for j := hunkStart; j < i; j++ {
					if j < len(existingLines) {
						hunkLines = append(hunkLines, " "+existingLines[j])
					}
				}
			}
			
			// Add removed line
			if i < len(existingLines) && existingLine != "" {
				hunkLines = append(hunkLines, "-"+existingLine)
			}
			// Add added line
			if i < len(newLines) && newLine != "" {
				hunkLines = append(hunkLines, "+"+newLine)
			}
		} else if inHunk {
			// Add context line
			hunkLines = append(hunkLines, " "+existingLine)
			
			// Check if we should end the hunk (after 3 context lines)
			contextCount := 0
			for j := len(hunkLines) - 1; j >= 0 && strings.HasPrefix(hunkLines[j], " "); j-- {
				contextCount++
			}
			
			if contextCount >= 3 {
				// End hunk
				writeHunk(&diff, hunkStart, i-contextCount+1, len(existingLines), len(newLines), hunkLines[:len(hunkLines)-contextCount+3])
				inHunk = false
			}
		}
	}
	
	// Close any remaining hunk
	if inHunk {
		writeHunk(&diff, hunkStart, maxLines, len(existingLines), len(newLines), hunkLines)
	}
	
	return diff.String()
}

func writeHunk(diff *bytes.Buffer, start, end, oldLen, newLen int, lines []string) {
	oldCount := 0
	newCount := 0
	
	for _, line := range lines {
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, " ") {
			oldCount++
		}
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, " ") {
			newCount++
		}
	}
	
	diff.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", start+1, oldCount, start+1, newCount))
	for _, line := range lines {
		diff.WriteString(line + "\n")
	}
}

func generateShortHash(content string) string {
	// Simple hash simulation (first 7 chars of a basic hash)
	hash := 0
	for _, c := range content {
		hash = hash*31 + int(c)
	}
	if hash < 0 {
		hash = -hash
	}
	return fmt.Sprintf("%07x", hash%0xfffffff)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var diffTool string
var diffToolDetected bool

func detectDiffTool() string {
	if diffToolDetected {
		return diffTool
	}
	
	diffToolDetected = true
	
	// Check for diff tools in order of preference
	tools := []string{"delta", "difftastic", "diff-so-fancy"}
	
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err == nil {
			diffTool = tool
			return diffTool
		}
	}
	
	// No fancy diff tool found
	diffTool = ""
	return diffTool
}

func outputDiff(diffContent string) {
	tool := detectDiffTool()
	
	if tool == "" {
		// No external tool, output directly
		fmt.Print(diffContent)
		return
	}
	
	// Pipe diff content to external tool
	var cmd *exec.Cmd
	
	switch tool {
	case "delta":
		cmd = exec.Command("delta", "--no-gitconfig", "--side-by-side")
	case "difftastic":
		cmd = exec.Command("difftastic", "--display=side-by-side")
	case "diff-so-fancy":
		cmd = exec.Command("diff-so-fancy")
	default:
		// Fallback
		fmt.Print(diffContent)
		return
	}
	
	cmd.Stdin = strings.NewReader(diffContent)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		// If external tool fails, fallback to plain output
		fmt.Print(diffContent)
	}
}