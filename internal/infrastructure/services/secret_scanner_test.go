//go:build unit

package services_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	services "github.com/rios0rios0/aisync/internal/infrastructure/services"
)

func TestRegexSecretScanner_Scan_ShouldDetectAWSAccessKeyID(t *testing.T) {
	// given
	files := map[string][]byte{
		"config.yaml": []byte("aws_key: AKIAIOSFODNN7EXAMPLE"),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.Len(t, findings, 1)
	assert.Equal(t, "config.yaml", findings[0].Path)
	assert.Equal(t, "AWS Access Key ID", findings[0].Description)
}

func TestRegexSecretScanner_Scan_ShouldDetectGitHubPAT(t *testing.T) {
	// given
	files := map[string][]byte{
		"env.sh": []byte("export TOKEN=ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij"),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.Len(t, findings, 1)
	assert.Equal(t, "GitHub Personal Access Token", findings[0].Description)
}

func TestRegexSecretScanner_Scan_ShouldDetectAnthropicAPIKey(t *testing.T) {
	// given
	key := "sk-ant-" + generateRepeatedString("abcdefABCDEF0123456789-_", 90)
	files := map[string][]byte{
		"secrets.env": []byte("ANTHROPIC_KEY=" + key),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.GreaterOrEqual(t, len(findings), 1)

	found := false
	for _, f := range findings {
		if f.Description == "Anthropic API Key" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have detected Anthropic API Key")
}

func TestRegexSecretScanner_Scan_ShouldDetectOpenAIAPIKey(t *testing.T) {
	// given
	key := "sk-" + generateRepeatedString("abcdefABCDEF0123456789", 48)
	files := map[string][]byte{
		"config.json": []byte(`{"api_key": "` + key + `"}`),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.GreaterOrEqual(t, len(findings), 1)

	found := false
	for _, f := range findings {
		if f.Description == "OpenAI API Key" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have detected OpenAI API Key")
}

func TestRegexSecretScanner_Scan_ShouldDetectPrivateKeyHeader(t *testing.T) {
	// given
	files := map[string][]byte{
		"key.pem": []byte("-----BEGIN RSA PRIVATE KEY-----\nMIIBogIBAAJ..."),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.Len(t, findings, 1)
	assert.Equal(t, "Private Key", findings[0].Description)
}

func TestRegexSecretScanner_Scan_ShouldDetectAgeSecretKey(t *testing.T) {
	// given
	ageKey := "AGE-SECRET-KEY-" + generateRepeatedString("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", 59)
	files := map[string][]byte{
		"identity.txt": []byte(ageKey),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.Len(t, findings, 1)
	assert.Equal(t, "age Private Key", findings[0].Description)
}

func TestRegexSecretScanner_Scan_ShouldDetectJWTToken(t *testing.T) {
	// given
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	files := map[string][]byte{
		"auth.json": []byte(`{"token": "` + jwt + `"}`),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.Len(t, findings, 1)
	assert.Equal(t, "JWT Token", findings[0].Description)
}

func TestRegexSecretScanner_Scan_ShouldDetectSlackToken(t *testing.T) {
	// given
	files := map[string][]byte{
		"slack.env": []byte("SLACK_TOKEN=xoxb-1234567890-abcdefghij"),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.Len(t, findings, 1)
	assert.Equal(t, "Slack Token", findings[0].Description)
}

func TestRegexSecretScanner_Scan_ShouldReturnNoFindingsForCleanFile(t *testing.T) {
	// given
	files := map[string][]byte{
		"readme.md":   []byte("# My Project\nThis is a clean readme file."),
		"main.go":     []byte("package main\n\nfunc main() {}\n"),
		"config.yaml": []byte("debug: true\nport: 8080\n"),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.Len(t, findings, 0)
}

func TestRegexSecretScanner_Scan_ShouldReturnCorrectLineNumbers(t *testing.T) {
	// given
	content := "line one\nline two\nAKIAIOSFODNN7EXAMPLE\nline four\n"
	files := map[string][]byte{
		"test.txt": []byte(content),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.Len(t, findings, 1)
	assert.Equal(t, 3, findings[0].Line)
}

func TestRegexSecretScanner_Scan_ShouldDetectMultipleFindingsInSameFile(t *testing.T) {
	// given
	content := "key1: AKIAIOSFODNN7EXAMPLE\n" +
		"-----BEGIN RSA PRIVATE KEY-----\n" +
		"token: xoxb-1234567890-abcdefghij\n"
	files := map[string][]byte{
		"multi.env": []byte(content),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.Len(t, findings, 3)

	descriptions := make([]string, len(findings))
	for i, f := range findings {
		descriptions[i] = f.Description
	}
	assert.Contains(t, descriptions, "AWS Access Key ID")
	assert.Contains(t, descriptions, "Private Key")
	assert.Contains(t, descriptions, "Slack Token")
}

func TestRegexSecretScanner_Scan_ShouldDetectGoogleAPIKey(t *testing.T) {
	// given
	files := map[string][]byte{
		"config.json": []byte(`{"api_key": "AIzaSyA1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6Q"}`),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.Len(t, findings, 1)
	assert.Equal(t, "Google API Key", findings[0].Description)
}

func TestRegexSecretScanner_Scan_ShouldDetectConnectionString(t *testing.T) {
	// given
	files := map[string][]byte{
		"db.env": []byte(`connection_string = 'Server=db;Database=app;User=admin;Password=s3cret'`),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.GreaterOrEqual(t, len(findings), 1)

	found := false
	for _, f := range findings {
		if f.Description == "Connection String" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have detected Connection String")
}

func TestRegexSecretScanner_Scan_ShouldDetectHardcodedPassword(t *testing.T) {
	// given
	files := map[string][]byte{
		"settings.py": []byte(`password = "mysuperSecretPass123"`),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.GreaterOrEqual(t, len(findings), 1)

	found := false
	for _, f := range findings {
		if f.Description == "Hardcoded Password" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have detected Hardcoded Password")
}

func TestRegexSecretScanner_Scan_ShouldDetectMultipleDifferentSecretTypesInOneFile(t *testing.T) {
	// given
	googleKey := "AIzaSyA1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6Q"
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	content := "GOOGLE_KEY=" + googleKey + "\n" +
		"TOKEN=" + jwt + "\n" +
		`password = "verylongpassword123"` + "\n" +
		"SLACK=xoxb-1234567890-abcdefghij\n"
	files := map[string][]byte{
		"multi-secrets.env": []byte(content),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.GreaterOrEqual(t, len(findings), 4)

	descriptions := make(map[string]bool)
	for _, f := range findings {
		descriptions[f.Description] = true
	}
	assert.True(t, descriptions["Google API Key"], "should detect Google API Key")
	assert.True(t, descriptions["JWT Token"], "should detect JWT Token")
	assert.True(t, descriptions["Hardcoded Password"], "should detect Hardcoded Password")
	assert.True(t, descriptions["Slack Token"], "should detect Slack Token")
}

func TestRegexSecretScanner_Scan_ShouldDetectAWSSecretAccessKey(t *testing.T) {
	// given
	secretKey := generateRepeatedString("abcdefABCDEF0123456789/+=", 40)
	files := map[string][]byte{
		"aws.env": []byte("aws_secret_access_key = " + secretKey),
	}
	scanner := services.NewRegexSecretScanner()

	// when
	findings := scanner.Scan(files)

	// then
	assert.GreaterOrEqual(t, len(findings), 1)

	found := false
	for _, f := range findings {
		if f.Description == "AWS Secret Access Key" {
			found = true
			break
		}
	}
	assert.True(t, found, "should have detected AWS Secret Access Key")
}

// generateRepeatedString repeats characters from the alphabet to build a string of the given length.
func generateRepeatedString(alphabet string, length int) string {
	result := make([]byte, length)
	for i := range result {
		result[i] = alphabet[i%len(alphabet)]
	}
	return string(result)
}
