package vaultsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type VaultClient struct {
	Address   string
	Token     string
	Namespace string
	client    *http.Client
	Output    io.Writer
	ErrOutput io.Writer
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

type SecretRef struct {
	Engine string
	Path   string
}

var ErrSecretNotFound = errors.New("vault secret not found")

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

func metadataToDataPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && parts[1] == "metadata" {
		return parts[0] + "/data/" + strings.Join(parts[2:], "/")
	}
	return path
}

func metadataSubPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) <= 2 {
		return ""
	}
	return strings.Join(parts[2:], "/")
}

func NewSecretRef(engine, path string) SecretRef {
	return SecretRef{
		Engine: strings.Trim(strings.TrimSpace(engine), "/"),
		Path:   strings.Trim(strings.TrimSpace(path), "/"),
	}
}

func (r SecretRef) MetadataPath() string {
	if r.Path == "" {
		return r.Engine + "/metadata"
	}

	return r.Engine + "/metadata/" + r.Path
}

func secretRefFromMetadataPath(path string) SecretRef {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return SecretRef{}
	}

	parts := strings.Split(path, "/")
	if len(parts) >= 3 && parts[1] == "metadata" {
		return NewSecretRef(parts[0], strings.Join(parts[2:], "/"))
	}

	return NewSecretRef(parts[0], strings.Join(parts[1:], "/"))
}

func NewVaultClient(address, token, namespace string) *VaultClient {
	return &VaultClient{
		Address:   address,
		Token:     token,
		Namespace: namespace,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		Output:    os.Stdout,
		ErrOutput: os.Stderr,
	}
}

func NewVaultClientFromEnv(namespace string) (*VaultClient, error) {
	vaultAddr := os.Getenv("VAULT_ADDR")
	if vaultAddr == "" {
		return nil, fmt.Errorf("VAULT_ADDR environment variable is required")
	}

	vaultToken := os.Getenv("VAULT_TOKEN")
	if vaultToken == "" {
		return nil, fmt.Errorf("VAULT_TOKEN environment variable is required")
	}

	return NewVaultClient(vaultAddr, vaultToken, namespace), nil
}

// Deprecated: use NewSecretRef(kvEngine, subPath).MetadataPath() instead.
func BuildMetadataPath(kvEngine, subPath string) string {
	return NewSecretRef(kvEngine, subPath).MetadataPath()
}

func (v *VaultClient) output() io.Writer {
	if v.Output == nil {
		return io.Discard
	}
	return v.Output
}

func (v *VaultClient) errOutput() io.Writer {
	if v.ErrOutput == nil {
		return io.Discard
	}
	return v.ErrOutput
}

func (v *VaultClient) printf(format string, args ...interface{}) {
	fmt.Fprintf(v.output(), format, args...)
}

func (v *VaultClient) ListSecretsAt(ref SecretRef) ([]string, error) {
	kvPath := ref.MetadataPath()
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

// Deprecated: use ListSecretsAt with SecretRef instead.
func (v *VaultClient) ListSecrets(kvPath string) ([]string, error) {
	return v.ListSecretsAt(secretRefFromMetadataPath(kvPath))
}

func (v *VaultClient) GetSecretAt(ref SecretRef) (map[string]interface{}, error) {
	secretPath := ref.MetadataPath()
	dataPath := metadataToDataPath(secretPath)
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
		httpErr := &HTTPError{StatusCode: resp.StatusCode, Body: string(body)}
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %s", ErrSecretNotFound, httpErr)
		}
		return nil, httpErr
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

// Deprecated: use GetSecretAt with SecretRef instead.
func (v *VaultClient) GetSecret(secretPath string) (map[string]interface{}, error) {
	return v.GetSecretAt(secretRefFromMetadataPath(secretPath))
}

func (v *VaultClient) PullSecretsRecursivelyAt(ref SecretRef) (map[string]map[string]interface{}, error) {
	secrets := make(map[string]map[string]interface{})

	return v.pullSecretsRecursivelyHelper(ref.MetadataPath(), secrets)
}

// Deprecated: use PullSecretsRecursivelyAt with SecretRef instead.
func (v *VaultClient) PullSecretsRecursively(basePath string) (map[string]map[string]interface{}, error) {
	return v.PullSecretsRecursivelyAt(secretRefFromMetadataPath(basePath))
}

func (v *VaultClient) pullSecretsRecursivelyHelper(currentPath string, secrets map[string]map[string]interface{}) (map[string]map[string]interface{}, error) {
	keys, err := v.ListSecretsAt(secretRefFromMetadataPath(currentPath))
	if err != nil {
		return secrets, fmt.Errorf("failed to list secrets at %s: %w", currentPath, err)
	}

	var resultErr error

	for _, key := range keys {
		fullPath := currentPath + "/" + key

		// If key ends with /, it's a folder - recurse into it
		if key[len(key)-1] == '/' {
			folderPath := currentPath + "/" + key[:len(key)-1]
			secrets, err = v.pullSecretsRecursivelyHelper(folderPath, secrets)
			if err != nil {
				resultErr = errors.Join(resultErr, err)
				continue
			}
		} else {
			// It's a secret - fetch its data
			secretPath := currentPath + "/" + key

			secretData, err := v.GetSecretAt(secretRefFromMetadataPath(secretPath))
			if err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("failed to get secret %s: %w", fullPath, err))
				continue
			}
			secrets[fullPath] = secretData
		}
	}

	return secrets, resultErr
}

func (v *VaultClient) PullSecretsToFilesAt(ref SecretRef, outputDir string) error {
	return v.pullSecretsToFiles(ref.MetadataPath(), outputDir, true, ".yaml")
}

// Deprecated: use PullSecretsToFilesAt with SecretRef instead.
func (v *VaultClient) PullSecretsToFiles(basePath, outputDir string) error {
	return v.PullSecretsToFilesAt(secretRefFromMetadataPath(basePath), outputDir)
}

func (v *VaultClient) PullSecretsToFilesDirectAt(ref SecretRef, outputDir string) error {
	return v.pullSecretsToFiles(ref.MetadataPath(), outputDir, false, "")
}

// Deprecated: use PullSecretsToFilesDirectAt with SecretRef instead.
func (v *VaultClient) PullSecretsToFilesDirect(basePath, outputDir string) error {
	return v.PullSecretsToFilesDirectAt(secretRefFromMetadataPath(basePath), outputDir)
}

func (v *VaultClient) pullSecretsToFiles(basePath, outputDir string, mirrorBasePath bool, fileExtension string) error {
	secrets, pullErr := v.PullSecretsRecursivelyAt(secretRefFromMetadataPath(basePath))
	if pullErr != nil {
		pullErr = fmt.Errorf("failed to pull secrets: %w", pullErr)
	}

	secretPaths := make([]string, 0, len(secrets))
	for secretPath := range secrets {
		secretPaths = append(secretPaths, secretPath)
	}
	slices.Sort(secretPaths)

	for _, secretPath := range secretPaths {
		secretData := secrets[secretPath]
		if err := v.writeSecretToFile(secretPath, secretData, basePath, outputDir, mirrorBasePath, fileExtension); err != nil {
			writeErr := fmt.Errorf("failed to write secret %s: %w", secretPath, err)
			if pullErr != nil {
				return errors.Join(writeErr, pullErr)
			}
			return writeErr
		}
	}

	return pullErr
}

func (v *VaultClient) writeSecretToFile(secretPath string, secretData map[string]interface{}, metadataPath, outputDir string, mirrorBasePath bool, fileExtension string) error {
	// Extract the relative path from the secret path
	relativePath := strings.TrimPrefix(secretPath, metadataPath)
	relativePath = strings.TrimPrefix(relativePath, "/") // Remove leading slash if present

	if relativePath == "" {
		// Handle edge case where secret name would be empty
		return fmt.Errorf("cannot determine file name for secret %s", secretPath)
	}

	// Extract subpath from metadataPath to determine target directory
	targetDir := outputDir
	if mirrorBasePath {
		if subPath := metadataSubPath(metadataPath); subPath != "" {
			targetDir = filepath.Join(outputDir, subPath)
		}
	}

	// Create file path with optional extension
	filePath := filepath.Join(targetDir, relativePath+fileExtension)

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

	v.printf("Written: %s\n", filePath)
	return nil
}

func (v *VaultClient) PutSecretAt(ref SecretRef, secretData map[string]interface{}) error {
	secretPath := ref.MetadataPath()
	dataPath := metadataToDataPath(secretPath)

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
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	return nil
}

// Deprecated: use PutSecretAt with SecretRef instead.
func (v *VaultClient) PutSecret(secretPath string, secretData map[string]interface{}) error {
	return v.PutSecretAt(secretRefFromMetadataPath(secretPath), secretData)
}

func (v *VaultClient) PushSecretsFromFilesAt(inputDir string, ref SecretRef, dryRun bool) error {
	return v.pushSecretsFromFiles(inputDir, ref.MetadataPath(), dryRun, true, ".yaml")
}

// Deprecated: use PushSecretsFromFilesAt with SecretRef instead.
func (v *VaultClient) PushSecretsFromFiles(inputDir, metadataPath string, dryRun bool) error {
	return v.PushSecretsFromFilesAt(inputDir, secretRefFromMetadataPath(metadataPath), dryRun)
}

func (v *VaultClient) PushSecretsFromFilesDirectAt(inputDir string, ref SecretRef, dryRun bool) error {
	return v.pushSecretsFromFiles(inputDir, ref.MetadataPath(), dryRun, false, "")
}

// Deprecated: use PushSecretsFromFilesDirectAt with SecretRef instead.
func (v *VaultClient) PushSecretsFromFilesDirect(inputDir, metadataPath string, dryRun bool) error {
	return v.PushSecretsFromFilesDirectAt(inputDir, secretRefFromMetadataPath(metadataPath), dryRun)
}

func shouldProcessSecretFile(filePath string, fileExtension string) bool {
	if fileExtension == "" {
		return true
	}

	return strings.HasSuffix(filePath, fileExtension)
}

func trimSecretFileExtension(path string, fileExtension string) string {
	if fileExtension == "" {
		return path
	}

	return strings.TrimSuffix(path, fileExtension)
}

func (v *VaultClient) pushSecretsFromFiles(inputDir, metadataPath string, dryRun bool, mirrorBasePath bool, fileExtension string) error {
	var baseDir string

	// Extract the KV engine name and subpath
	parts := strings.Split(metadataPath, "/")
	if len(parts) < 2 || parts[1] != "metadata" {
		return fmt.Errorf("invalid metadata path: %s", metadataPath)
	}

	kvEngine := parts[0]
	subPath := metadataSubPath(metadataPath)
	if mirrorBasePath && subPath != "" {
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

		// Skip directories and files outside the configured secret format.
		if info.IsDir() || !shouldProcessSecretFile(filePath, fileExtension) {
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

		// Remove any configured file extension and convert to vault path.
		secretPath := trimSecretFileExtension(relativePath, fileExtension)
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
			v.printf("Pushing: %s\n", vaultPath)
			return v.PutSecretAt(secretRefFromMetadataPath(vaultPath), secretData)
		}
	})
}

func (v *VaultClient) showDryRunDiff(vaultPath string, newData map[string]interface{}) error {
	// Try to get existing secret
	existingData, err := v.GetSecretAt(secretRefFromMetadataPath(vaultPath))
	secretMissing := false

	var existingYaml []byte
	if err != nil {
		if !errors.Is(err, ErrSecretNotFound) {
			return fmt.Errorf("failed to get existing secret %s: %w", vaultPath, err)
		}

		// Secret doesn't exist, use empty content
		secretMissing = true
		existingYaml = []byte{}
	} else {
		var marshalErr error
		existingYaml, marshalErr = yaml.Marshal(existingData)
		if marshalErr != nil {
			return fmt.Errorf("failed to marshal existing secret %s: %w", vaultPath, marshalErr)
		}
	}

	newYaml, err := yaml.Marshal(newData)
	if err != nil {
		return fmt.Errorf("failed to marshal new secret %s: %w", vaultPath, err)
	}

	// Generate unified diff
	var diffOutput string

	if secretMissing {
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
		outputDiff(diffOutput, v.output(), v.errOutput())
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

func outputDiff(diffContent string, stdout, stderr io.Writer) {
	tool := detectDiffTool()

	if tool == "" {
		// No external tool, output directly
		fmt.Fprint(stdout, diffContent)
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
		fmt.Fprint(stdout, diffContent)
		return
	}

	cmd.Stdin = strings.NewReader(diffContent)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		// If external tool fails, fallback to plain output
		fmt.Fprint(stdout, diffContent)
	}
}
