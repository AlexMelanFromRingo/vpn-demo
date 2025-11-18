// +build windows

package tun

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

const (
	// Latest Wintun release
	wintunVersion = "0.14.1"
	wintunURL     = "https://www.wintun.net/builds/wintun-" + wintunVersion + ".zip"
)

// EnsureWintun checks if wintun.dll exists and downloads it if needed
func EnsureWintun() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	exeDir := filepath.Dir(exePath)
	dllPath := filepath.Join(exeDir, "wintun.dll")

	// Check if already exists
	if _, err := os.Stat(dllPath); err == nil {
		return nil // Already exists
	}

	fmt.Println("wintun.dll not found, downloading...")
	return downloadWintun(dllPath)
}

// downloadWintun downloads and extracts wintun.dll
func downloadWintun(dstPath string) error {
	// Create temp file for zip
	tmpZip, err := os.CreateTemp("", "wintun-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpZip.Name())
	defer tmpZip.Close()

	// Download
	fmt.Printf("Downloading Wintun %s...\n", wintunVersion)
	resp, err := http.Get(wintunURL)
	if err != nil {
		return fmt.Errorf("failed to download wintun: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to download wintun: HTTP %d", resp.StatusCode)
	}

	// Save to temp file
	if _, err := io.Copy(tmpZip, resp.Body); err != nil {
		return fmt.Errorf("failed to save wintun: %w", err)
	}

	tmpZip.Close()

	// Extract wintun.dll
	if err := extractWintunDLL(tmpZip.Name(), dstPath); err != nil {
		return fmt.Errorf("failed to extract wintun.dll: %w", err)
	}

	fmt.Printf("wintun.dll installed to: %s\n", dstPath)
	return nil
}

// extractWintunDLL extracts wintun.dll from the zip archive
func extractWintunDLL(zipPath, dstPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	// Determine architecture
	arch := "amd64"
	if runtime.GOARCH == "386" {
		arch = "x86"
	} else if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}

	// Find wintun.dll for current architecture
	targetFile := fmt.Sprintf("wintun/bin/%s/wintun.dll", arch)

	for _, f := range r.File {
		if f.Name == targetFile {
			return extractFile(f, dstPath)
		}
	}

	return fmt.Errorf("wintun.dll for %s not found in archive", arch)
}

// extractFile extracts a single file from zip
func extractFile(f *zip.File, dstPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, rc)
	return err
}
