package main

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
)

const (
	// SectionDivider marks the beginning of a file section
	SectionDivider = "--------"

	// EndMarker indicates the end of the repository content
	EndMarker = "----END----"
)

// VCS directories to auto-exclude by default
var vcsDirectories = []string{
	".git/",
	".svn/",
	".hg/",
	".bzr/",
	"CVS/",
	".darcs/",
}

// Global warning counter
var warningCount int

// Version information (set by build process)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var header = fmt.Sprintf(`This text describes a repository with code. It consists of sections starting with %s, followed by a line with the file path and name, then varying lines of file contents. The repository text concludes when %s is reached. Any text after %s is to be understood as instructions related to the provided repository.`, SectionDivider, EndMarker, EndMarker)

// IgnorePattern represents a single ignore pattern with its directory context
type IgnorePattern struct {
	Pattern   string // The actual pattern (e.g., "*.log", "temp/")
	Dir       string // The directory where this pattern was found (relative to root)
	IsNegated bool   // Whether this pattern is negated (starts with !)
}

// Config holds the program configuration
type Config struct {
	Directory             string
	Output                string
	OutputPath            string
	IncludeVCSDirectories bool
}

// exitWithError prints an error message and exits with code 1
func exitWithError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

// printWarning prints a warning message and increments the warning counter
func printWarning(format string, args ...interface{}) {
	warningCount++
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
}

func main() {
	cmd := &cli.Command{
		Name:    "unfolder",
		Usage:   "Convert repository contents to text format for AI analysis",
		Version: fmt.Sprintf("%s (%s) %s", version, commit, date),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "include-vcs",
				Usage:   "Include VCS directories (.git/, .svn/, etc.) in output",
				Aliases: []string{"vcs"},
			},
		},
		Action: run,
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		exitWithError("%v", err)
	}
}

// run is the main application logic
func run(ctx context.Context, c *cli.Command) error {
	args := c.Args().Slice()

	// Parse positional arguments
	var directory, output string
	switch len(args) {
	case 0:
		directory = "."
	case 1:
		directory = args[0]
	case 2:
		directory = args[0]
		output = args[1]
	default:
		return cli.Exit("Too many arguments", 1)
	}

	// Create config
	config := &Config{
		Directory:             directory,
		Output:                output,
		IncludeVCSDirectories: c.Bool("include-vcs"),
	}

	// Determine output file path
	outputPath, err := determineOutputPath(config.Directory, config.Output)
	if err != nil {
		return cli.Exit(fmt.Sprintf("Error determining output path: %v", err), 1)
	}
	config.OutputPath = outputPath

	// Process the repository
	if err := processRepository(config.Directory, config.OutputPath, config); err != nil {
		return cli.Exit(fmt.Sprintf("%v", err), 1)
	}

	// Write --END-- marker
	if err := writeEnd(config.OutputPath); err != nil {
		printWarning("Could not write end marker: %v", err)
	}

	fmt.Printf("Repository contents written to %s\n", config.OutputPath)

	// Show warning summary if any warnings occurred
	if warningCount > 0 {
		fmt.Fprintf(os.Stderr, "\nNote: %d warning(s) occurred during processing. Some files may have been skipped due to permission issues.\n", warningCount)
	}

	return nil
}

func determineOutputPath(directory, output string) (string, error) {
	// Get the base directory name
	absDir, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	baseName := filepath.Base(absDir)
	defaultFilename := baseName + ".txt"

	// No output specified, use current directory
	if output == "" {
		return defaultFilename, nil
	}

	// Output is a directory
	if strings.HasSuffix(output, "/") || strings.HasSuffix(output, "\\") {
		return filepath.Join(output, defaultFilename), nil
	}

	// Output is a file path
	return output, nil
}

func processRepository(directory, outputPath string, config *Config) error {
	// Load ignore patterns
	ignorePatterns, err := loadIgnorePatterns(directory)
	if err != nil {
		return err
	}

	// Get absolute paths
	absDir, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return err
	}

	// Create output file and write header
	output, err := createOutputFile(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()

	// Walk through files
	return walkAndProcessFiles(absDir, absOutput, ignorePatterns, output, config)
}

// createOutputFile creates the output file and writes the header
func createOutputFile(outputPath string) (*os.File, error) {
	output, err := os.Create(outputPath)
	if err != nil {
		return nil, err
	}

	// Write header
	fmt.Fprintln(output, header)
	return output, nil
}

// walkAndProcessFiles walks through the directory and processes each file
func walkAndProcessFiles(absDir, absOutput string, ignorePatterns []IgnorePattern, output *os.File, config *Config) error {
	return filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Handle permission errors for directories
			if os.IsPermission(err) {
				printWarning("Permission denied accessing %s: %v", path, err)
				return filepath.SkipDir // Skip this directory and its contents
			}
			return err
		}

		// Get relative path for ignore checking
		relPath, err := filepath.Rel(absDir, path)
		if err != nil {
			printWarning("Could not get relative path for %s: %v", path, err)
			return nil
		}

		// Check if directory should be ignored (before entering it)
		if d.IsDir() {
			if shouldIgnore(relPath, ignorePatterns, config) {
				return filepath.SkipDir // Skip this directory and its contents
			}
			return nil // Continue into this directory
		}

		// For files, process normally
		return processDirectoryEntry(path, d, absDir, absOutput, ignorePatterns, output, config)
	})
}

// processDirectoryEntry processes a single directory entry (file or subdirectory)
func processDirectoryEntry(path string, d fs.DirEntry, absDir, absOutput string, ignorePatterns []IgnorePattern, output *os.File, config *Config) error {
	// Skip if it's the output file itself
	if absPath, _ := filepath.Abs(path); absPath == absOutput {
		return nil
	}

	// Skip symlinks
	if d.Type()&fs.ModeSymlink != 0 {
		return nil
	}

	// Get relative path
	relPath, err := filepath.Rel(absDir, path)
	if err != nil {
		// This is unusual, but continue processing
		printWarning("Could not get relative path for %s: %v", path, err)
		return nil
	}

	// Check if file should be ignored
	if shouldIgnore(relPath, ignorePatterns, config) {
		return nil
	}

	// Check if file is binary
	if isBinary(path) {
		return nil
	}

	// Process file
	return processFile(path, relPath, output)
}

func loadIgnorePatterns(directory string) ([]IgnorePattern, error) {
	var patterns []IgnorePattern

	// Get absolute path for the root directory
	absDir, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}

	// Load ignore patterns incrementally, respecting already-loaded patterns
	err = loadIgnorePatternsRecursive(absDir, "", &patterns)
	return patterns, err
}

// loadIgnorePatternsRecursive loads ignore patterns recursively, respecting already-loaded patterns
func loadIgnorePatternsRecursive(absDir, relDir string, patterns *[]IgnorePattern) error {
	// Build current path
	currentPath := absDir
	if relDir != "" {
		currentPath = filepath.Join(absDir, relDir)
	}

	// Check if current directory should be ignored based on already-loaded patterns
	if relDir != "" && shouldIgnore(relDir, *patterns, &Config{IncludeVCSDirectories: false}) {
		return nil // Skip this directory entirely
	}

	// Read .gitignore and .unfolderignore files in current directory
	ignoreFiles := []string{".gitignore", ".unfolderignore"}
	for _, ignoreFile := range ignoreFiles {
		ignorePath := filepath.Join(currentPath, ignoreFile)
		if filePatterns, err := readIgnoreFileWithContext(ignorePath, relDir); err == nil {
			*patterns = append(*patterns, filePatterns...)
		}
	}

	// List directory contents
	entries, err := os.ReadDir(currentPath)
	if err != nil {
		if os.IsPermission(err) {
			printWarning("Permission denied accessing %s: %v", currentPath, err)
			return nil
		}
		return err
	}

	// Process subdirectories
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip VCS directories
		if !shouldIgnore(entry.Name(), *patterns, &Config{IncludeVCSDirectories: false}) {
			// Build relative path for subdirectory
			subRelDir := entry.Name()
			if relDir != "" {
				subRelDir = filepath.Join(relDir, entry.Name())
			}

			// Recursively load patterns from subdirectory
			if err := loadIgnorePatternsRecursive(absDir, subRelDir, patterns); err != nil {
				return err
			}
		}
	}

	return nil
}

func readIgnoreFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		// Check if it's a permission error
		if os.IsPermission(err) {
			printWarning("Permission denied reading %s: %v", path, err)
			return nil, nil // Return empty patterns, continue processing
		}
		return nil, err
	}
	defer file.Close()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, line)
		}
	}

	return patterns, scanner.Err()
}

func readIgnoreFileWithContext(path, ignoreDir string) ([]IgnorePattern, error) {
	file, err := os.Open(path)
	if err != nil {
		// Check if it's a permission error
		if os.IsPermission(err) {
			printWarning("Permission denied reading %s: %v", path, err)
			return nil, nil // Return empty patterns, continue processing
		}
		return nil, err
	}
	defer file.Close()

	var patterns []IgnorePattern
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line != "" && !strings.HasPrefix(line, "#") {
			isNegated := strings.HasPrefix(line, "!")
			pattern := line
			if isNegated {
				pattern = strings.TrimPrefix(line, "!")
			}

			patterns = append(patterns, IgnorePattern{
				Pattern:   pattern,
				Dir:       ignoreDir,
				IsNegated: isNegated,
			})
		}
	}

	return patterns, scanner.Err()
}

func shouldIgnore(filePath string, patterns []IgnorePattern, config *Config) bool {
	// Check VCS directories first (unless explicitly included)
	if !config.IncludeVCSDirectories {
		for _, vcsDir := range vcsDirectories {
			// Check if the path contains a VCS directory anywhere in the path
			// This handles cases like "baserow/.git/HEAD" or "project/.svn/entries"
			pathParts := strings.Split(filepath.ToSlash(filePath), "/")
			for _, part := range pathParts {
				if part == strings.TrimSuffix(vcsDir, "/") {
					return true
				}
			}
		}
	}

	// Check user-defined patterns with Git-like behavior
	// Each .gitignore affects its own directory and sub-directories
	for _, pattern := range patterns {
		// Check if this pattern applies to the current file path
		if isPatternApplicable(filePath, pattern) {
			if pattern.IsNegated {
				// Negated patterns override previous ignore decisions
				return false
			} else {
				// Regular ignore pattern
				return true
			}
		}
	}
	return false
}

// isPatternApplicable checks if a pattern from a specific directory applies to the given file path
func isPatternApplicable(filePath string, pattern IgnorePattern) bool {
	// Convert paths to forward slashes for consistent matching
	filePath = filepath.ToSlash(filePath)
	patternDir := filepath.ToSlash(pattern.Dir)
	patternText := filepath.ToSlash(pattern.Pattern)

	// If the pattern is from the root directory (empty dir), it applies to all files
	if patternDir == "" {
		return matchPattern(filePath, patternText)
	}

	// Check if the file path is within the directory where this pattern was defined
	// or in a subdirectory of that directory
	if !strings.HasPrefix(filePath, patternDir+"/") && filePath != patternDir {
		return false
	}

	// For patterns defined in a subdirectory, we need to check if the pattern
	// matches the relative path from that directory
	if patternDir != "" {
		// Get the relative path from the pattern's directory
		relPath := filePath
		if strings.HasPrefix(filePath, patternDir+"/") {
			relPath = filePath[len(patternDir+"/"):]
		}
		return matchPattern(relPath, patternText)
	}

	return matchPattern(filePath, patternText)
}

// Enhanced pattern matching for gitignore patterns
func matchPattern(filePath, pattern string) bool {
	// Remove leading slash
	pattern = strings.TrimPrefix(pattern, "/")
	filePath = strings.TrimPrefix(filePath, "/")

	// Convert to forward slashes for consistent matching
	filePath = filepath.ToSlash(filePath)
	pattern = filepath.ToSlash(pattern)

	// Handle negation (patterns starting with !)
	if strings.HasPrefix(pattern, "!") {
		return false // Negation not supported in this context
	}

	// Handle double asterisk patterns (/**/)
	if strings.Contains(pattern, "/**/") {
		return matchDoubleAsterisk(filePath, pattern)
	}

	// Handle patterns ending with /**
	if strings.HasSuffix(pattern, "/**") {
		basePattern := strings.TrimSuffix(pattern, "/**")
		return strings.HasPrefix(filePath, basePattern+"/") || filePath == basePattern
	}

	// Handle patterns starting with **/
	if strings.HasPrefix(pattern, "**/") {
		basePattern := strings.TrimPrefix(pattern, "**/")
		return strings.HasSuffix(filePath, "/"+basePattern) || filePath == basePattern
	}

	// Exact match
	if pattern == filePath {
		return true
	}

	// Directory pattern (ends with /)
	if strings.HasSuffix(pattern, "/") {
		dirPattern := strings.TrimSuffix(pattern, "/")
		return strings.HasPrefix(filePath, dirPattern+"/") || filePath == dirPattern
	}

	// Enhanced wildcard patterns
	if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") || strings.Contains(pattern, "[") {
		return enhancedWildcardMatch(filePath, pattern)
	}

	// Prefix match for directories
	return strings.HasPrefix(filePath, pattern+"/")
}

// matchDoubleAsterisk handles /**/ patterns
func matchDoubleAsterisk(filePath, pattern string) bool {
	parts := strings.Split(pattern, "/**/")
	if len(parts) != 2 {
		return false
	}

	prefix := parts[0]
	suffix := parts[1]

	// If prefix is empty, just check suffix
	if prefix == "" {
		return strings.HasSuffix(filePath, "/"+suffix) || filePath == suffix
	}

	// If suffix is empty, just check prefix
	if suffix == "" {
		return strings.HasPrefix(filePath, prefix+"/") || filePath == prefix
	}

	// Check both prefix and suffix
	if !strings.HasPrefix(filePath, prefix) {
		return false
	}

	// Find suffix after prefix
	remaining := filePath[len(prefix):]
	return strings.HasSuffix(remaining, "/"+suffix) || remaining == "/"+suffix
}

// enhancedWildcardMatch handles *, ?, and character classes
func enhancedWildcardMatch(text, pattern string) bool {
	// Convert pattern to regex-like matching
	return matchWildcardPattern(text, pattern)
}

// matchWildcardPattern implements enhanced wildcard matching
func matchWildcardPattern(text, pattern string) bool {
	// Handle simple cases first
	if pattern == "*" {
		return true
	}
	if pattern == "?" {
		return len(text) == 1
	}

	// Convert pattern to regex-like matching
	return matchPatternRecursive(text, pattern)
}

// matchPatternRecursive recursively matches pattern against text
func matchPatternRecursive(text, pattern string) bool {
	// Base cases
	if pattern == "" {
		return text == ""
	}
	if text == "" {
		return pattern == "" || pattern == "*"
	}

	// Handle different pattern characters
	switch pattern[0] {
	case '*':
		// * can match zero or more characters
		if len(pattern) == 1 {
			return true // * at end matches everything
		}
		// Try matching * with 0, 1, 2, ... characters
		for i := 0; i <= len(text); i++ {
			if matchPatternRecursive(text[i:], pattern[1:]) {
				return true
			}
		}
		return false

	case '?':
		// ? matches exactly one character
		return matchPatternRecursive(text[1:], pattern[1:])

	case '[':
		// Character class
		end := strings.Index(pattern, "]")
		if end == -1 {
			return false // Malformed character class
		}
		charClass := pattern[1:end]
		remainingPattern := pattern[end+1:]

		// Check if current character matches the class
		if len(text) == 0 {
			return false
		}
		if !matchCharacterClass(text[0], charClass) {
			return false
		}
		return matchPatternRecursive(text[1:], remainingPattern)

	default:
		// Literal character
		if text[0] != pattern[0] {
			return false
		}
		return matchPatternRecursive(text[1:], pattern[1:])
	}
}

// matchCharacterClass checks if a character matches a character class
func matchCharacterClass(c byte, charClass string) bool {
	if len(charClass) == 0 {
		return false
	}

	// Handle negation
	negated := false
	if charClass[0] == '!' {
		negated = true
		charClass = charClass[1:]
	}

	// Handle ranges like a-z
	for i := 0; i < len(charClass); i++ {
		if i+2 < len(charClass) && charClass[i+1] == '-' {
			start := charClass[i]
			end := charClass[i+2]
			if c >= start && c <= end {
				return !negated
			}
			i += 2 // Skip the range
		} else {
			if c == charClass[i] {
				return !negated
			}
		}
	}

	return negated
}

func isBinary(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		// Check if it's a permission error
		if os.IsPermission(err) {
			printWarning("Permission denied reading %s: %v", path, err)
			return true // Assume binary if can't read due to permissions
		}
		return true // Assume binary if can't read
	}
	defer file.Close()

	// Read first 512 bytes
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && n == 0 {
		return true
	}

	// Check for null bytes
	for i := 0; i < n; i++ {
		if buffer[i] == 0 {
			return true
		}
	}

	return false
}

func processFile(path, relPath string, output *os.File) error {
	content, err := os.ReadFile(path)
	if err != nil {
		// Check if it's a permission error
		if os.IsPermission(err) {
			printWarning("Permission denied reading %s: %v", path, err)
			return nil // Skip this file, continue processing
		}
		return err
	}

	// Write section separator
	fmt.Fprintln(output, SectionDivider)

	// Write file path
	fmt.Fprintln(output, relPath)

	// Write file contents
	fmt.Fprint(output, string(content))

	// Ensure newline after content
	if len(content) > 0 && content[len(content)-1] != '\n' {
		fmt.Fprintln(output)
	}

	return nil
}

func writeEnd(outputPath string) error {
	file, err := os.OpenFile(outputPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		// Check if it's a permission error
		if os.IsPermission(err) {
			printWarning("Permission denied writing to %s: %v", outputPath, err)
			return err // This is a critical error, return it
		}
		return err
	}
	defer file.Close()

	_, err = fmt.Fprintln(file, EndMarker)
	return err
}
