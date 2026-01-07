package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

const (
	installer = "https://instl.vercel.app"
	repo      = "tgdrive/teldrive"
	windowsOS = "windows"
)

type scriptExecutor struct {
	platformType string
	shellCmd     string
	shellArgs    []string
}

func executeScript(e scriptExecutor) error {

	executable, err := os.Executable()

	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	executableDir := filepath.Dir(executable)

	executableName := filepath.Base(executable)

	if e.platformType == "windows" {
		oldPath := filepath.Join(executableDir, executableName+".old")
		_ = os.Remove(oldPath)
		if err := os.Rename(executable, oldPath); err != nil {
			return fmt.Errorf("failed to rename executable: %v", err)
		}
	}

	url := fmt.Sprintf("%s/%s?type=script&move=0", installer, repo)
	if e.platformType == "windows" {
		url += "&platform=windows"
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch script: %v", err)
	}
	defer resp.Body.Close()

	scriptContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read script: %v", err)
	}

	cmd := exec.Command(e.shellCmd, e.shellArgs...)
	cmd.Dir = executableDir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %v", err)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %v", err)
	}

	go func() {
		defer stdin.Close()
		stdin.Write(scriptContent)
	}()

	if err := cmd.Wait(); err != nil {
		if e.platformType == "windows" {
			oldPath := filepath.Join(executableDir, executableName+".old")
			_ = os.Rename(oldPath, executable)
		}
		return fmt.Errorf("script execution failed: %v", err)
	}

	if e.platformType == "windows" {
		go func() {
			oldPath := filepath.Join(executableDir, executableName+".old")
			_ = os.Remove(oldPath)
		}()
	}

	return nil
}

func checkVersion() error {
	cmd := exec.Command("teldrive", "version")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func NewUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade TelDrive",
		Long:  "Upgrade TelDrive to the latest version.",
		Run: func(cmd *cobra.Command, args []string) {
			var executor scriptExecutor

			switch runtime.GOOS {
			case "windows":
				executor = scriptExecutor{
					platformType: "windows",
					shellCmd:     "powershell",
					shellArgs:    []string{"-NoProfile", "-NonInteractive", "-Command", "-"},
				}
			case "darwin", "linux":
				executor = scriptExecutor{
					platformType: "unix",
					shellCmd:     "bash",
					shellArgs:    []string{},
				}
			default:
				fmt.Fprintf(os.Stderr, "Unsupported operating system: %s\n", runtime.GOOS)
				os.Exit(1)
			}

			if err := executeScript(executor); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if runtime.GOOS != "windows" {
				if err := checkVersion(); err != nil {
					fmt.Fprintf(os.Stderr, "Error checking version: %v\n", err)
					os.Exit(1)
				}
			} else {
				fmt.Println("Restart TelDrive to use the new version.")
			}
		},
	}
}
