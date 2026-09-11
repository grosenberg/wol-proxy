package testutil

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// RootDir returns the absolute path to the directory containing go.mod.
func RootDir() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	for {
		key := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(key); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("go.mod not found in any parent directory")
		}
		dir = parent
	}
}

// InitLogging for an individual test:
// changes the test working directory to the project
// root and configures slog.
func InitLogging(t *testing.T, logpathname string) {
	t.Helper()
	t.Chdir(RootDir())
	if logpathname != "" {
		file := slogInit(logpathname)
		t.Cleanup(func() {
			file.Close()
		})
	}
}

// InitMainLogging (from TestMain) for all package tests:
// changes the test working directory to the project
// root and configures slog.
// Example TestMain code:
//     func TestMain(m *testing.M) {
// 	        cleanup := testutil.InitMainLogging("wol-proxy-test.log")
// 	        defer cleanup()
// 	        os.Exit(m.Run())
//     }

func InitMainLogging(logPathname string) func() {
	if err := os.Chdir(RootDir()); err != nil {
		panic("Failed to change working directory to root: " + err.Error())
	}
	if logPathname != "" {
		file := slogInit(logPathname)
		return func() {
			file.Close()
		}
	}
	return func() {}
}

func slogInit(logPathname string) *os.File {
	file, err := os.OpenFile(logPathname, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic("Failed to open log file: " + err.Error())
	}

	// Formats time to "YYYY-MM-DD hh:mm:ss.mmm"
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Formats time to "YYYY-MM-DD hh:mm:ss.mmm"
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05.000"))
			}
			// Reduces the source path to just the base name
			if a.Key == slog.SourceKey {
				source, _ := a.Value.Any().(*slog.Source)
				if source != nil {
					source.File = filepath.Base(source.File)
				}
			}
			return a
		},
	}

	handler := slog.NewTextHandler(file, opts)
	slog.SetDefault(slog.New(handler))
	return file
}

func CopyFile(src, dst string) error {
	// Open the source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Create the destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	// Copy the content
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Flush file metadata to disk
	err = dstFile.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to get source file info: %w", err)
	}

	err = os.Chmod(dst, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	return nil
}
