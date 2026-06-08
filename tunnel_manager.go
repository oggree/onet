package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

func getCloudflaredURL() (string, error) {
	osName := runtime.GOOS
	arch := runtime.GOARCH

	var binaryName string

	if osName == "linux" && arch == "amd64" {
		binaryName = "cloudflared-linux-amd64"
	} else if osName == "linux" && arch == "arm64" {
		binaryName = "cloudflared-linux-arm64"
	} else if osName == "darwin" && arch == "amd64" {
		binaryName = "cloudflared-darwin-amd64"
	} else if osName == "darwin" && arch == "arm64" {
		binaryName = "cloudflared-darwin-arm64"
	} else {
		return "", fmt.Errorf("unsupported OS/Arch for auto-download: %s/%s", osName, arch)
	}

	return fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/latest/download/%s", binaryName), nil
}

func ensureCloudflared() (string, error) {
	// Let's store it in the same directory as the executable or in a specific hidden folder.
	// For simplicity, let's use the current working directory / bin folder.
	binDir := "bin"
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", err
	}

	binaryPath := filepath.Join(binDir, "cloudflared")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}

	if _, err := os.Stat(binaryPath); err == nil {
		return binaryPath, nil
	}

	log.Println("cloudflared binary not found. Downloading...")

	url, err := getCloudflaredURL()
	if err != nil {
		return "", err
	}

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download cloudflared: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("bad status code while downloading: %d", resp.StatusCode)
	}

	out, err := os.Create(binaryPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", err
	}

	// Make executable
	if runtime.GOOS != "windows" {
		if err := os.Chmod(binaryPath, 0755); err != nil {
			return "", err
		}
	}

	log.Println("Successfully downloaded cloudflared.")
	return binaryPath, nil
}

func StartCloudflared(token string, stopChan <-chan struct{}) {
	binaryPath, err := ensureCloudflared()
	if err != nil {
		log.Printf("Failed to setup cloudflared: %v\n", err)
		log.Println("Please install cloudflared manually or check the OS/Arch compatibility.")
		return
	}

	for {
		log.Println("Starting cloudflared tunnel...")
		cmd := exec.Command(binaryPath, "tunnel", "--no-autoupdate", "run", "--token", token)

		// Map stdout/stderr to our application's stdout/stderr
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			log.Printf("Failed to start cloudflared process: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case <-stopChan:
			log.Println("Stopping cloudflared tunnel...")
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			return
		case err := <-done:
			log.Printf("cloudflared process exited: %v", err)
		}

		log.Println("Restarting cloudflared in 5 seconds...")
		time.Sleep(5 * time.Second)
	}
}
