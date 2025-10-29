package phpenv

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"phpenv/internal/config"
)

type ThreadSafety string

const (
	ThreadSafe    ThreadSafety = "ts"
	NonThreadSafe ThreadSafety = "nts"
)

type Manager struct {
	cfg *config.Config
}

type InstallOptions struct {
	Version       string
	Arch          string
	ThreadSafety  ThreadSafety
	ForceDownload bool
	Progress      ProgressCallback
}

type RemoveOptions struct {
	Version      string
	Arch         string
	ThreadSafety ThreadSafety
}

type UseOptions struct {
	Version      string
	Arch         string
	ThreadSafety ThreadSafety
	CustomPath   string
	Description  string
}

type LocalOptions struct {
	Dir           string
	Selection     config.Selection
	Env           map[string]string
	PathAdditions []string
	Clear         bool
}

type ResolveOptions struct {
	WorkingDir  string
	ForceGlobal bool
}

type ProgressCallback func(DownloadProgress)

type DownloadProgress struct {
	Stage     string
	Total     int64
	Completed int64
	Done      bool
	Err       error
}

type RemoteFilter struct {
	Arch         string
	ThreadSafety ThreadSafety
}

type LocalVersion struct {
	Name         string
	Path         string
	PHPPath      string
	Active       bool
	Version      string
	ThreadSafety ThreadSafety
	Arch         string
}

type ResolvedRuntime struct {
	PHPPath       string
	Selection     config.Selection
	Source        string // "local" or "global"
	Env           map[string]string
	PathAdditions []string
	VersionSlug   string
}

type WinBuild struct {
	Version string
	NTS     bool
	Arch    string
	URL     string
}

var buildRe = regexp.MustCompile(`php-(\d+\.\d+\.\d+(?:-RC\d+)?)((?:-nts)?)-(?:Win32|win32)-([a-zA-Z0-9]+)-(x64|x86)\.zip`)

func NewManager(cfg *config.Config) *Manager {
	return &Manager{cfg: cfg}
}

func (m *Manager) Config() *config.Config {
	return m.cfg
}

func (m *Manager) ListLocal() ([]LocalVersion, error) {
	dir := m.cfg.VersionsDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []LocalVersion{}, nil
		}
		return nil, err
	}

	activeSlug := strings.ToLower(strings.TrimSpace(m.cfg.Global.Version))
	var result []LocalVersion
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		phpPath := filepath.Join(dir, name, "php.exe")
		if !fileExists(phpPath) {
			continue
		}
		version, ts, arch := parseMetadata(name)
		result = append(result, LocalVersion{
			Name:         name,
			Path:         filepath.Join(dir, name),
			PHPPath:      phpPath,
			Active:       activeSlug != "" && strings.EqualFold(activeSlug, name),
			Version:      version,
			ThreadSafety: ts,
			Arch:         arch,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

func (m *Manager) ListRemote(filter RemoteFilter) ([]WinBuild, error) {
	builds, err := fetchWindowsBuilds()
	if err != nil {
		return nil, err
	}
	var result []WinBuild
	for _, b := range builds {
		if filter.Arch != "" && !strings.EqualFold(filter.Arch, b.Arch) {
			continue
		}
		if filter.ThreadSafety != "" {
			wantNTS := filter.ThreadSafety == NonThreadSafe
			if b.NTS != wantNTS {
				continue
			}
		}
		result = append(result, b)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Version == result[j].Version {
			if result[i].NTS == result[j].NTS {
				return result[i].Arch < result[j].Arch
			}
			return result[i].NTS && !result[j].NTS
		}
		return result[i].Version > result[j].Version
	})
	return result, nil
}

func (m *Manager) Install(opts InstallOptions) error {
	if opts.Version == "" {
		return errors.New("version is required")
	}
	ts := normalizeThreadSafety(opts.ThreadSafety, m.cfg.DefaultThreadSafety)
	arch := normalizeArch(opts.Arch, m.cfg.DefaultArch)

	build, err := selectBuild(opts.Version, arch, ts)
	if err != nil {
		return err
	}
	if err := m.cfg.EnsureDirs(); err != nil {
		return err
	}

	cacheZip := filepath.Join(m.cfg.CacheDir, filepath.Base(build.URL))
	if opts.ForceDownload || !fileExists(cacheZip) {
		if err := downloadFile(build.URL, cacheZip, opts.Progress); err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
	} else if opts.Progress != nil {
		opts.Progress(DownloadProgress{
			Stage:     "cache",
			Total:     1,
			Completed: 1,
			Done:      true,
		})
	}

	target := filepath.Join(m.cfg.VersionsDir, strings.TrimSuffix(filepath.Base(build.URL), ".zip"))
	if dirExists(target) {
		return fmt.Errorf("version already installed: %s", target)
	}
	if err := unzip(cacheZip, target); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}
	return nil
}

func (m *Manager) Remove(opts RemoveOptions) error {
	if opts.Version == "" {
		return errors.New("version is required")
	}
	ts := normalizeThreadSafety(opts.ThreadSafety, m.cfg.DefaultThreadSafety)
	arch := normalizeArch(opts.Arch, m.cfg.DefaultArch)

	dir, err := m.findInstalled(opts.Version, arch, ts)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	slug := filepath.Base(dir)
	if strings.EqualFold(m.cfg.Global.Version, slug) {
		m.cfg.Global = config.Selection{}
		if err := m.cfg.Save(); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Use(opts UseOptions) error {
	var selection config.Selection
	if opts.CustomPath != "" {
		phpPath := normalizeLocalPath(opts.CustomPath)
		if !fileExists(phpPath) {
			return fmt.Errorf("custom php path not found: %s", phpPath)
		}
		selection = config.Selection{
			CustomPHP:   phpPath,
			Description: strings.TrimSpace(opts.Description),
		}
	} else {
		if opts.Version == "" {
			return errors.New("version is required")
		}
		ts := normalizeThreadSafety(opts.ThreadSafety, m.cfg.DefaultThreadSafety)
		arch := normalizeArch(opts.Arch, m.cfg.DefaultArch)
		dir, err := m.findInstalled(opts.Version, arch, ts)
		if err != nil {
			return err
		}
		selection = config.Selection{
			Version:     filepath.Base(dir),
			Description: strings.TrimSpace(opts.Description),
		}
	}
	if err := m.setGlobalSelection(selection); err != nil {
		return err
	}
	if err := m.ensureShim(); err != nil {
		return err
	}
	return m.applySystemEnvironment()
}

func (m *Manager) SetLocal(opts LocalOptions) error {
	dir := opts.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if opts.Clear {
		path := filepath.Join(dir, ".phpenv.json")
		if fileExists(path) {
			return os.Remove(path)
		}
		return nil
	}
	local := &config.LocalConfig{
		Use:           opts.Selection,
		Env:           opts.Env,
		PathAdditions: opts.PathAdditions,
	}
	if local.Env == nil {
		local.Env = map[string]string{}
	}
	return config.SaveLocal(dir, local)
}

func (m *Manager) Resolve(opts ResolveOptions) (ResolvedRuntime, error) {
	working := opts.WorkingDir
	if working == "" {
		var err error
		working, err = os.Getwd()
		if err != nil {
			return ResolvedRuntime{}, err
		}
	}
	working, _ = filepath.Abs(working)

	var localCfg *config.LocalConfig
	if !opts.ForceGlobal {
		if cfg, err := config.FindLocal(working); err == nil {
			localCfg = cfg
		}
	}

	selection := config.Selection{}
	source := "global"
	if localCfg != nil && !isSelectionEmpty(localCfg.Use) {
		selection = localCfg.Use
		source = "local"
	} else if !isSelectionEmpty(m.cfg.Global) {
		selection = m.cfg.Global
	}

	if isSelectionEmpty(selection) {
		return ResolvedRuntime{}, errors.New("no active PHP selected; run `phpenv use`")
	}

	env := cloneEnv(m.cfg.Env)
	pathAdditions := append([]string(nil), m.cfg.PathAdditions...)

	if selection.Version != "" {
		if profile, ok := m.cfg.VersionProfiles[selection.Version]; ok {
			mergeEnv(env, profile.Env)
			pathAdditions = append(pathAdditions, profile.PathAdditions...)
		}
	}
	if localCfg != nil {
		mergeEnv(env, localCfg.Env)
		pathAdditions = append(pathAdditions, localCfg.PathAdditions...)
	}

	var phpPath string
	if selection.CustomPHP != "" {
		phpPath = normalizeLocalPath(selection.CustomPHP)
		if !fileExists(phpPath) {
			return ResolvedRuntime{}, fmt.Errorf("custom php not found: %s", phpPath)
		}
	} else {
		if selection.Version == "" {
			return ResolvedRuntime{}, errors.New("missing version selection")
		}
		phpPath = filepath.Join(m.cfg.VersionsDir, selection.Version, "php.exe")
		if !fileExists(phpPath) {
			return ResolvedRuntime{}, fmt.Errorf("php.exe not found for version %s", selection.Version)
		}
	}

	phpDir := filepath.Dir(phpPath)
	pathAdditions = append([]string{phpDir}, pathAdditions...)

	env["PHPENV_HOME"] = m.cfg.Root
	env["PHPENV_ROOT"] = m.cfg.Root
	env["PHPENV_PHP"] = phpPath
	if selection.Version != "" {
		env["PHPENV_VERSION"] = selection.Version
	} else {
		env["PHPENV_VERSION"] = "custom"
	}

	return ResolvedRuntime{
		PHPPath:       phpPath,
		Selection:     selection,
		Source:        source,
		Env:           env,
		PathAdditions: dedupePaths(pathAdditions),
		VersionSlug:   selection.Version,
	}, nil
}

func (m *Manager) Which(dir string) (ResolvedRuntime, error) {
	return m.Resolve(ResolveOptions{WorkingDir: dir})
}

func (m *Manager) ensureShim() error {
	if err := m.cfg.EnsureDirs(); err != nil {
		return err
	}
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath = normalizeLocalPath(exePath)
	shimPath := filepath.Join(m.cfg.ShimsDir, "php.cmd")
	content := fmt.Sprintf("@echo off\r\n\"%s\" exec php %%*\r\nexit /b %%ERRORLEVEL%%\r\n", escapeBatchValue(exePath))
	return os.WriteFile(shimPath, []byte(content), 0o755)
}

func (m *Manager) setGlobalSelection(sel config.Selection) error {
	normalised, err := sanitizeSelection(sel)
	if err != nil {
		return err
	}
	m.cfg.Global = normalised
	if err := m.cfg.Save(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) findInstalled(version, arch string, ts ThreadSafety) (string, error) {
	dir := m.cfg.VersionsDir
	if !dirExists(dir) {
		return "", errors.New("no versions installed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	tsStr := string(ts)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.Contains(name, "php-"+version+"-") &&
			strings.Contains(strings.ToLower(name), "-"+tsStr+"-") &&
			strings.HasSuffix(strings.ToLower(name), "-"+strings.ToLower(arch)) {
			return filepath.Join(dir, name), nil
		}
	}
	return "", fmt.Errorf("version %s (%s-%s) is not installed", version, tsStr, arch)
}

func selectBuild(version, arch string, ts ThreadSafety) (*WinBuild, error) {
	builds, err := fetchWindowsBuilds()
	if err != nil {
		return nil, err
	}
	wantNTS := ts == NonThreadSafe
	for _, b := range builds {
		if b.Version == version && strings.EqualFold(b.Arch, arch) && b.NTS == wantNTS {
			return &b, nil
		}
	}
	return nil, fmt.Errorf("no matching build found for version %s (%s-%s)", version, ts, arch)
}

func fetchWindowsBuilds() ([]WinBuild, error) {
	urls := []string{
		"https://windows.php.net/downloads/releases/",
		"https://windows.php.net/downloads/releases/archives/",
	}
	seen := map[string]WinBuild{}
	for _, u := range urls {
		html, err := httpGet(u)
		if err != nil {
			continue
		}
		matches := buildRe.FindAllStringSubmatch(html, -1)
		for _, match := range matches {
			version := match[1]
			nts := strings.Contains(match[2], "nts")
			arch := strings.ToLower(match[4])
			href := match[0]
			url := href
			if !strings.HasPrefix(href, "http") {
				url = strings.TrimSuffix(u, "/") + "/" + href
			}
			key := fmt.Sprintf("%s|%t|%s", version, nts, arch)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = WinBuild{
				Version: version,
				NTS:     nts,
				Arch:    arch,
				URL:     url,
			}
		}
	}
	if len(seen) == 0 {
		return nil, errors.New("no builds found from windows.php.net")
	}
	out := make([]WinBuild, 0, len(seen))
	for _, build := range seen {
		out = append(out, build)
	}
	return out, nil
}

func httpGet(url string) (string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "phpenv/0.2 (+https://github.com/alaad/phpenv)")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func downloadFile(url, dst string, progress ProgressCallback) error {
	tmp := dst + ".part"
	_ = os.Remove(tmp)
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer out.Close()

	client := &http.Client{Timeout: 0}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "phpenv/0.2")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download http %d", resp.StatusCode)
	}

	total := resp.ContentLength
	if progress != nil {
		progress(DownloadProgress{Stage: "download", Total: total, Completed: 0})
	}

	reader := &progressReader{
		ReadCloser: resp.Body,
		total:      total,
		callback:   progress,
	}
	if _, err := io.Copy(out, reader); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if progress != nil {
		progress(DownloadProgress{Stage: "download", Total: total, Completed: reader.read, Done: true})
	}
	return os.Rename(tmp, dst)
}

type progressReader struct {
	io.ReadCloser
	total    int64
	read     int64
	callback ProgressCallback
	lastTick time.Time
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.read += int64(n)
	if r.callback != nil {
		now := time.Now()
		if now.Sub(r.lastTick) > 150*time.Millisecond || err != nil {
			r.callback(DownloadProgress{
				Stage:     "download",
				Total:     r.total,
				Completed: r.read,
				Done:      err == io.EOF,
				Err:       err,
			})
			r.lastTick = now
		}
	}
	return n, err
}

func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		fpath := filepath.Join(dst, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(fpath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return err
		}
		rc.Close()
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}

func normalizeThreadSafety(ts ThreadSafety, fallback string) ThreadSafety {
	if ts == "" {
		ts = ThreadSafety(strings.ToLower(fallback))
	}
	switch strings.ToLower(string(ts)) {
	case "ts":
		return ThreadSafe
	case "nts":
		return NonThreadSafe
	default:
		return NonThreadSafe
	}
}

func normalizeArch(arch, fallback string) string {
	if arch == "" {
		arch = fallback
	}
	if arch == "" {
		arch = "x64"
	}
	return strings.ToLower(arch)
}

func parseMetadata(name string) (string, ThreadSafety, string) {
	if m := buildRe.FindStringSubmatch(name + ".zip"); len(m) == 5 {
		version := m[1]
		nts := strings.Contains(m[2], "nts")
		arch := strings.ToLower(m[4])
		if nts {
			return version, NonThreadSafe, arch
		}
		return version, ThreadSafe, arch
	}
	parts := strings.Split(name, "-")
	if len(parts) >= 2 {
		version := parts[1]
		ts := NonThreadSafe
		for _, p := range parts {
			if strings.EqualFold(p, "ts") {
				ts = ThreadSafe
			}
		}
		arch := parts[len(parts)-1]
		return version, ts, strings.ToLower(arch)
	}
	return name, NonThreadSafe, ""
}

func isSelectionEmpty(sel config.Selection) bool {
	return strings.TrimSpace(sel.Version) == "" && strings.TrimSpace(sel.CustomPHP) == ""
}

func sanitizeSelection(sel config.Selection) (config.Selection, error) {
	sel.Version = strings.TrimSpace(sel.Version)
	sel.Description = strings.TrimSpace(sel.Description)
	sel.CustomPHP = normalizeLocalPath(sel.CustomPHP)
	if sel.Version != "" && sel.CustomPHP != "" {
		return sel, errors.New("selection cannot contain both version and custom path")
	}
	return sel, nil
}

func cloneEnv(src map[string]string) map[string]string {
	dst := map[string]string{}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mergeEnv(base map[string]string, overrides map[string]string) {
	for k, v := range overrides {
		if k == "" {
			continue
		}
		base[k] = v
	}
}

func dedupePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, p := range paths {
		if p == "" {
			continue
		}
		n := normalizeLocalPath(p)
		key := strings.ToLower(n)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, n)
	}
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func normalizeLocalPath(p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
	}
	return filepath.Clean(p)
}

func escapeBatchValue(val string) string {
	val = strings.ReplaceAll(val, "\"", "\"\"")
	val = strings.ReplaceAll(val, "\r", "")
	val = strings.ReplaceAll(val, "\n", "")
	return val
}
