package e2e

import (
	"fmt"
	"testing"
)

// TestSendReceiveFile covers single-file transfers across a range of sizes,
// including boundary cases around the 64 KB chunk boundary.
func TestSendReceiveFile(t *testing.T) {
	const chunk = 64 * 1024

	tests := []struct {
		name string
		size int
	}{
		{"empty", 0},
		{"one byte", 1},
		{"small ascii", 512},
		{"just under chunk", chunk - 1},
		{"exact chunk", chunk},
		{"just over chunk", chunk + 1},
		{"two chunks", chunk * 2},
		{"fuzzy multi-chunk", 200*1024 + 37},
		{"one megabyte", 1024 * 1024},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			newHarness(t, "") // random nameplate; each sub-test owns its servers

			content := randContent(tc.size)
			src := makeSourceFile(t, "payload.bin", content)
			doSendReceiveFile(t, src, content)
		})
	}
}

// TestSendReceiveDir covers directory transfers: flat, nested, many files,
// and a fuzzy case with random file counts and sizes.
func TestSendReceiveDir(t *testing.T) {
	tests := []struct {
		name    string
		dirName string
		files   []fileSpec
	}{
		{
			name:    "single file",
			dirName: "single",
			files: []fileSpec{
				{path: "hello.txt", content: []byte("hello world")},
			},
		},
		{
			name:    "multiple flat files",
			dirName: "flat",
			files: []fileSpec{
				{path: "a.txt", content: randContent(100)},
				{path: "b.txt", content: randContent(200)},
				{path: "c.bin", content: randContent(50)},
			},
		},
		{
			name:    "nested directories",
			dirName: "nested",
			files: []fileSpec{
				{path: "root.txt", content: randContent(80)},
				{path: "sub/a.txt", content: randContent(120)},
				{path: "sub/b.txt", content: randContent(90)},
				{path: "deep/nested/c.bin", content: randContent(60)},
			},
		},
		{
			name:    "mixed sizes",
			dirName: "mixed",
			files: []fileSpec{
				{path: "tiny.txt", content: []byte("x")},
				{path: "small.bin", content: randContent(1024)},
				{path: "medium.bin", content: randContent(128 * 1024)},
				{path: "sub/large.bin", content: randContent(512 * 1024)},
			},
		},
		{
			name:    "many small files",
			dirName: "many",
			files:   manyFiles(20, 256),
		},
		{
			name:    "fuzzy random content",
			dirName: "fuzzy",
			files:   fuzzyFiles(15),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			newHarness(t, "")

			src := makeSourceDir(t, tc.dirName, tc.files)
			doSendReceiveDir(t, src, tc.files)
		})
	}
}

// manyFiles produces n files each containing size random bytes.
func manyFiles(n, size int) []fileSpec {
	files := make([]fileSpec, n)
	for i := range files {
		files[i] = fileSpec{
			path:    fmt.Sprintf("file%03d.bin", i),
			content: randContent(size),
		}
	}
	return files
}

// fuzzyFiles produces n files with random names and random sizes up to 4 KB.
func fuzzyFiles(n int) []fileSpec {
	files := make([]fileSpec, n)
	for i := range files {
		// Sizes: 0 B, 1 B, …, up to 4096 B, cycling through the range.
		size := (i * 317) % 4097 // coprime step gives varied sizes without crypto rand
		files[i] = fileSpec{
			path:    fmt.Sprintf("f%02d_%04d.bin", i, size),
			content: randContent(size),
		}
	}
	return files
}
