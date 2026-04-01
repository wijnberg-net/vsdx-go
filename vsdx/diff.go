package vsdx

import (
	"archive/zip"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

// VisioFileDiff compares two .vsdx files by examining their ZIP contents.
//
// It extracts the XML (and other text) files from both archives, compares
// their member lists, and computes line-by-line diffs for common members
// that have different content.
type VisioFileDiff struct {
	FilepathA string
	FilepathB string
	contentsA map[string][]string
	contentsB map[string][]string
	// Diffs contains the line-by-line diff for each common member that differs.
	// Each diff line is prefixed with "  " (same), "- " (only in A), or "+ " (only in B).
	Diffs map[string][]string
}

// NewVisioFileDiff creates a diff between two .vsdx files.
func NewVisioFileDiff(filepathA, filepathB string) (*VisioFileDiff, error) {
	if filepathA == filepathB {
		return nil, fmt.Errorf("the two file paths should be different")
	}
	if !strings.HasSuffix(strings.ToLower(filepathA), ".vsdx") ||
		!strings.HasSuffix(strings.ToLower(filepathB), ".vsdx") {
		return nil, fmt.Errorf("both files should be .vsdx files")
	}

	contentsA, err := extractFileData(filepathA)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %w", filepathA, err)
	}
	contentsB, err := extractFileData(filepathB)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %w", filepathB, err)
	}

	d := &VisioFileDiff{
		FilepathA: filepathA,
		FilepathB: filepathB,
		contentsA: contentsA,
		contentsB: contentsB,
	}
	d.Diffs = d.getFileDiffs()
	return d, nil
}

func (d *VisioFileDiff) String() string {
	return fmt.Sprintf("VisioFileDiff(a=%s, b=%s)", d.FilepathA, d.FilepathB)
}

// CommonMembers returns a sorted list of all member paths from both files (union).
func (d *VisioFileDiff) CommonMembers() []string {
	members := make(map[string]bool)
	for k := range d.contentsA {
		members[k] = true
	}
	for k := range d.contentsB {
		members[k] = true
	}
	result := make([]string, 0, len(members))
	for k := range members {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

// CompareMembers returns true if both files have exactly the same set of member paths.
func (d *VisioFileDiff) CompareMembers() bool {
	if len(d.contentsA) != len(d.contentsB) {
		return false
	}
	for k := range d.contentsA {
		if _, ok := d.contentsB[k]; !ok {
			return false
		}
	}
	return true
}

// AddedMembers returns member paths present in file B but not in file A.
func (d *VisioFileDiff) AddedMembers() []string {
	var result []string
	for k := range d.contentsB {
		if _, ok := d.contentsA[k]; !ok {
			result = append(result, k)
		}
	}
	sort.Strings(result)
	return result
}

// RemovedMembers returns member paths present in file A but not in file B.
func (d *VisioFileDiff) RemovedMembers() []string {
	var result []string
	for k := range d.contentsA {
		if _, ok := d.contentsB[k]; !ok {
			result = append(result, k)
		}
	}
	sort.Strings(result)
	return result
}

func (d *VisioFileDiff) getFileDiffs() map[string][]string {
	common := d.CommonMembers()
	diffs := make(map[string][]string)

	for _, member := range common {
		dataA := breakAllXMLIntoLines(d.contentsA[member])
		dataB := breakAllXMLIntoLines(d.contentsB[member])

		if dataA != nil && dataB != nil && !strSlicesEqual(dataA, dataB) {
			diff := computeDiff(dataA, dataB)
			diffs[member] = diff
		}
	}
	return diffs
}

// breakAllXMLIntoLines splits XML data into lines, splitting at '<' characters
// to make diffs more granular (element-level rather than line-level).
func breakAllXMLIntoLines(data []string) []string {
	if data == nil {
		return nil
	}
	var result []string
	for _, line := range data {
		parts := breakXMLIntoLines(line)
		result = append(result, parts...)
	}
	return result
}

// breakXMLIntoLines splits an XML line by inserting newlines before each '<'.
func breakXMLIntoLines(x string) []string {
	x = strings.ReplaceAll(x, "<", "\n<")
	return strings.Split(x, "\n")
}

// extractFileData reads a .vsdx file and returns its text contents as map of path -> lines.
// Binary files (non-UTF8) are stored with a placeholder string.
func extractFileData(filepath string) (map[string][]string, error) {
	r, err := zip.OpenReader(filepath)
	if err != nil {
		return nil, err
	}
	defer r.Close() //nolint:errcheck // best-effort close of ZIP reader

	contents := make(map[string][]string)
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			contents[f.Name] = []string{"Unable to read file."}
			continue
		}
		if !utf8.Valid(data) {
			contents[f.Name] = []string{"Unable to decode file."}
			continue
		}
		lines := strings.Split(string(data), "\n")
		contents[f.Name] = lines
	}
	return contents, nil
}

func strSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// computeDiff produces a line-by-line diff using LCS backtracking.
// Each line is prefixed with "  " (same), "- " (only in a), or "+ " (only in b).
func computeDiff(a, b []string) []string {
	m, n := len(a), len(b)

	// Build LCS DP table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Backtrack to produce diff
	var result []string
	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && a[i-1] == b[j-1] {
			result = append(result, "  "+a[i-1])
			i--
			j--
		} else if i > 0 && (j == 0 || dp[i-1][j] >= dp[i][j-1]) {
			result = append(result, "- "+a[i-1])
			i--
		} else {
			result = append(result, "+ "+b[j-1])
			j--
		}
	}

	// Reverse (backtracking produces lines in reverse order)
	for l, r := 0, len(result)-1; l < r; l, r = l+1, r-1 {
		result[l], result[r] = result[r], result[l]
	}

	return result
}
