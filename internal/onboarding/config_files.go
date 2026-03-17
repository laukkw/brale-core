package onboarding

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var errInvalidConfigPath = errors.New("invalid config file path")

type configTreeResult struct {
	Root  string            `json:"root"`
	Files []configFileEntry `json:"files"`
}

type configFileEntry struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Dir     string `json:"dir"`
	Ext     string `json:"ext"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

type configFileResult struct {
	Path    string `json:"path"`
	Ext     string `json:"ext"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
	Content string `json:"content"`
}

func loadConfigTree(repoRoot string) (configTreeResult, error) {
	root := filepath.Join(repoRoot, "configs")
	if _, err := os.Stat(root); err != nil {
		return configTreeResult{}, err
	}

	result := configTreeResult{Root: "configs + .env"}
	err := filepath.WalkDir(root, func(fullPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, fullPath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == "" {
			return nil
		}
		if isHiddenConfigPath(rel) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		dir := path.Dir(rel)
		if dir == "." {
			dir = ""
		}
		ext := strings.TrimPrefix(strings.ToLower(path.Ext(rel)), ".")
		result.Files = append(result.Files, configFileEntry{
			Path:    rel,
			Name:    path.Base(rel),
			Dir:     dir,
			Ext:     ext,
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		})
		return nil
	})
	if err != nil {
		return configTreeResult{}, err
	}

	envPath := filepath.Join(repoRoot, ".env")
	if info, envErr := os.Stat(envPath); envErr == nil && !info.IsDir() {
		result.Files = append(result.Files, configFileEntry{
			Path:    ".env",
			Name:    ".env",
			Dir:     "",
			Ext:     "env",
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		})
	}

	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].Path < result.Files[j].Path
	})
	return result, nil
}

func loadConfigFile(repoRoot, rawPath string) (configFileResult, error) {
	relPath, err := sanitizeConfigPath(rawPath)
	if err != nil {
		return configFileResult{}, err
	}
	if isHiddenConfigPath(relPath) {
		return configFileResult{}, errInvalidConfigPath
	}

	root := filepath.Join(repoRoot, "configs")
	fullPath := filepath.Join(root, filepath.FromSlash(relPath))
	scopeRoot := root
	if relPath == ".env" {
		fullPath = filepath.Join(repoRoot, ".env")
		scopeRoot = repoRoot
	}

	resolvedRoot, err := filepath.Abs(scopeRoot)
	if err != nil {
		return configFileResult{}, err
	}
	resolvedPath, err := filepath.Abs(fullPath)
	if err != nil {
		return configFileResult{}, err
	}
	if resolvedPath != resolvedRoot && !strings.HasPrefix(resolvedPath, resolvedRoot+string(os.PathSeparator)) {
		return configFileResult{}, errInvalidConfigPath
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return configFileResult{}, err
	}
	if info.IsDir() {
		return configFileResult{}, errInvalidConfigPath
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return configFileResult{}, err
	}

	ext := strings.TrimPrefix(strings.ToLower(path.Ext(relPath)), ".")
	return configFileResult{
		Path:    relPath,
		Ext:     ext,
		Size:    info.Size(),
		ModTime: info.ModTime().Format(time.RFC3339),
		Content: string(data),
	}, nil
}

func sanitizeConfigPath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", errInvalidConfigPath
	}
	if strings.ContainsRune(p, '\x00') {
		return "", errInvalidConfigPath
	}
	clean := path.Clean("/" + p)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." || strings.HasPrefix(clean, "../") {
		return "", errInvalidConfigPath
	}
	parts := strings.Split(clean, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", errInvalidConfigPath
		}
	}
	if strings.HasPrefix(clean, "/") {
		return "", errInvalidConfigPath
	}
	return clean, nil
}

func isHiddenConfigPath(relPath string) bool {
	clean := strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(relPath)), "/")
	if clean == "" || clean == "." {
		return false
	}
	first := strings.Split(clean, "/")[0]
	return first == "freqtrade" || first == "rules"
}
