package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"

	"phpenv/internal/config"
	"phpenv/internal/phpenv"
)

type flagHelp struct {
	Name        string
	Description string
}

type commandHelp struct {
	Name    string
	Usage   string
	Summary string
	Flags   []flagHelp
}

var commandCatalog = []commandHelp{
	{
		Name:    "help",
		Usage:   "help [command]",
		Summary: "Show general or command-specific help",
	},
	{
		Name:    "list",
		Usage:   "list [--remote] [--arch <x64|x86>] [--nts|--ts]",
		Summary: "List installed versions or remote builds",
		Flags: []flagHelp{
			{"--remote", "Show builds available on windows.php.net"},
			{"--arch", "Filter by architecture"},
			{"--nts", "Limit to non-thread-safe builds"},
			{"--ts", "Limit to thread-safe builds"},
		},
	},
	{
		Name:    "install",
		Usage:   "install <version> [--arch <x64|x86>] [--nts|--ts] [--force]",
		Summary: "Download and install a PHP build",
		Flags: []flagHelp{
			{"--arch", "Select architecture"},
			{"--nts", "Install non-thread-safe build"},
			{"--ts", "Install thread-safe build"},
			{"--force", "Re-download even if cached"},
		},
	},
	{
		Name:    "remove",
		Usage:   "remove <version> [--arch <x64|x86>] [--nts|--ts]",
		Summary: "Remove an installed PHP build",
		Flags: []flagHelp{
			{"--arch", "Select architecture"},
			{"--nts", "Remove non-thread-safe build"},
			{"--ts", "Remove thread-safe build"},
		},
	},
	{
		Name:    "use",
		Usage:   "use <version> [--arch <x64|x86>] [--nts|--ts] [--desc text] | use --path <php.exe>",
		Summary: "Activate an installed or custom PHP executable",
		Flags: []flagHelp{
			{"--arch", "Select architecture"},
			{"--nts", "Prefer non-thread-safe builds"},
			{"--ts", "Prefer thread-safe builds"},
			{"--desc", "Attach a friendly label"},
			{"--path", "Use a standalone php executable"},
		},
	},
	{
		Name:    "which",
		Usage:   "which [directory]",
		Summary: "Show the active php executable resolved for a directory",
	},
	{
		Name:    "config",
		Usage:   "config",
		Summary: "Print the path to the active config file",
	},
	{
		Name:    "exec",
		Usage:   "exec php [args...]",
		Summary: "Run php with the resolved environment",
	},
	{
		Name:    "local",
		Usage:   "local [--dir <path>] [--path <php.exe> | version] [--arch <x64|x86>] [--nts|--ts] [--env KEY=VAL] [--path-add PATH] [--clear]",
		Summary: "Manage per-directory overrides",
		Flags: []flagHelp{
			{"--dir", "Directory for the override file"},
			{"--path", "Custom php executable for this directory"},
			{"--env", "Add environment variable (repeatable)"},
			{"--path-add", "Prepend to PATH (repeatable)"},
			{"--clear", "Remove the override file"},
		},
	},
	{
		Name:    "menu",
		Usage:   "menu",
		Summary: "Launch the interactive terminal UI",
	},
}

var commandLookup = func() map[string]commandHelp {
	m := make(map[string]commandHelp)
	for _, c := range commandCatalog {
		m[c.Name] = c
	}
	return m
}()

func runCLI(a *App, args []string) error {
	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "help", "--help", "-h":
		if len(rest) == 0 {
			printUsage()
		} else {
			printCommandHelp(rest[0])
		}
		return nil
	case "list":
		return listCmd(a, rest)
	case "install":
		return installCmd(a, rest)
	case "remove":
		return removeCmd(a, rest)
	case "use":
		return useCmd(a, rest)
	case "which":
		return whichCmd(a, rest)
	case "config":
		printInfo(a.Config().FilePath())
		return nil
	case "menu":
		return a.runUI()
	case "exec":
		return execCmd(a, rest)
	case "local":
		return localCmd(a, rest)
	default:
		printError("Unknown command: %s", cmd)
		fmt.Println()
		printUsage()
		return nil
	}
}

func listCmd(a *App, args []string) error {
	manager := a.Manager()
	fs := newFlagSet("list")
	var (
		remote = fs.Bool("remote", false, "List remote builds from windows.php.net")
		arch   = fs.String("arch", "", "Filter by architecture")
		nts    = fs.Bool("nts", false, "Only show non-thread-safe builds")
		tsFlag = fs.Bool("ts", false, "Only show thread-safe builds")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *nts && *tsFlag {
		return errors.New("specify only one of --nts or --ts")
	}
	thread := phpenv.ThreadSafety("")
	if *nts {
		thread = phpenv.NonThreadSafe
	}
	if *tsFlag {
		thread = phpenv.ThreadSafe
	}
	if thread == "" {
		thread = defaultThreadSafety(a.Config())
	}

	if *remote {
		filter := phpenv.RemoteFilter{
			Arch:         normalizedArch(*arch, a.Config()),
			ThreadSafety: thread,
		}
		builds, err := manager.ListRemote(filter)
		if err != nil {
			return err
		}
		if len(builds) == 0 {
			printWarn("No remote builds match your filters.")
			return nil
		}
		printTitle("Remote PHP builds")
		for _, b := range builds {
			mode := "ts"
			if b.NTS {
				mode = "nts"
			}
			styleList.Printf("%-10s %-3s %-4s %s\n", b.Version, mode, b.Arch, b.URL)
		}
		return nil
	}

	local, err := manager.ListLocal()
	if err != nil {
		return err
	}
	if len(local) == 0 {
		printWarn("No versions installed yet.")
		return nil
	}
	for _, v := range local {
		style := styleList
		prefix := " "
		if v.Active {
			style = styleListBold
			prefix = "*"
		}
		label := fmt.Sprintf("%-10s %-3s %-4s %s", v.Version, strings.ToUpper(formatTS(v.ThreadSafety)), strings.ToUpper(v.Arch), v.Name)
		style.Printf("%s %s\n", prefix, label)
	}
	return nil
}

func installCmd(a *App, args []string) error {
	manager := a.Manager()
	fs := newFlagSet("install")
	var (
		arch  = fs.String("arch", "", "Architecture (x64 or x86)")
		nts   = fs.Bool("nts", false, "Install the non-thread-safe build")
		ts    = fs.Bool("ts", false, "Install the thread-safe build")
		force = fs.Bool("force", false, "Force re-download even if cached")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("install requires a version, e.g. 8.2.12")
	}
	if *nts && *ts {
		return errors.New("specify only one of --ts or --nts")
	}
	version := fs.Arg(0)
	thread := phpenv.ThreadSafety("")
	if *nts {
		thread = phpenv.NonThreadSafe
	}
	if *ts {
		thread = phpenv.ThreadSafe
	}
	if thread == "" {
		thread = defaultThreadSafety(a.Config())
	}

	printInfo("Installing PHP %s (%s)", version, formatTS(thread))
	showProgress := term.IsTerminal(int(os.Stdout.Fd()))
	var bar *progressbar.ProgressBar
	opts := phpenv.InstallOptions{
		Version:       version,
		Arch:          normalizedArch(*arch, a.Config()),
		ThreadSafety:  thread,
		ForceDownload: *force,
	}
	if showProgress {
		opts.Progress = func(dp phpenv.DownloadProgress) {
			if bar == nil {
				desc := "Downloading"
				if dp.Total <= 0 {
					bar = progressbar.NewOptions64(-1,
						progressbar.OptionSetDescription(desc),
						progressbar.OptionSpinnerType(14),
						progressbar.OptionSetRenderBlankState(true),
					)
				} else {
					bar = progressbar.NewOptions64(dp.Total,
						progressbar.OptionSetDescription(desc),
						progressbar.OptionShowBytes(true),
						progressbar.OptionSetWidth(40),
					)
				}
			}
			if dp.Total <= 0 {
				if dp.Done {
					_ = bar.Finish()
				} else {
					_ = bar.Add(1)
				}
				return
			}
			if dp.Completed > dp.Total {
				dp.Completed = dp.Total
			}
			_ = bar.Set64(dp.Completed)
			if dp.Done {
				_ = bar.Finish()
			}
		}
	}
	err := manager.Install(opts)
	if bar != nil {
		fmt.Println()
	}
	if err == nil {
		printSuccess("Installed PHP %s (%s)", version, formatTS(thread))
	}
	return err
}

func removeCmd(a *App, args []string) error {
	manager := a.Manager()
	fs := newFlagSet("remove")
	var (
		arch = fs.String("arch", "", "Architecture (x64 or x86)")
		nts  = fs.Bool("nts", false, "Remove the non-thread-safe build")
		ts   = fs.Bool("ts", false, "Remove the thread-safe build")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("remove requires a version, e.g. 8.2.12")
	}
	if *nts && *ts {
		return errors.New("specify only one of --ts or --nts")
	}
	thread := phpenv.ThreadSafety("")
	if *nts {
		thread = phpenv.NonThreadSafe
	}
	if *ts {
		thread = phpenv.ThreadSafe
	}
	if thread == "" {
		thread = defaultThreadSafety(a.Config())
	}
	printWarn("Removing PHP %s (%s)", fs.Arg(0), formatTS(thread))
	return manager.Remove(phpenv.RemoveOptions{
		Version:      fs.Arg(0),
		Arch:         normalizedArch(*arch, a.Config()),
		ThreadSafety: thread,
	})
}

func useCmd(a *App, args []string) error {
	manager := a.Manager()
	fs := newFlagSet("use")
	var (
		arch = fs.String("arch", "", "Architecture (x64 or x86)")
		nts  = fs.Bool("nts", false, "Prefer non-thread-safe build")
		ts   = fs.Bool("ts", false, "Prefer thread-safe build")
		path = fs.String("path", "", "Use a custom php executable")
		desc = fs.String("desc", "", "Attach a friendly description")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path != "" {
		if fs.NArg() > 0 {
			return errors.New("provide either a version or --path, not both")
		}
		return finishUse(manager, phpenv.UseOptions{
			CustomPath:  *path,
			Description: *desc,
		})
	}
	if fs.NArg() < 1 {
		return errors.New("use requires a version, e.g. 8.2.12")
	}
	if *nts && *ts {
		return errors.New("specify only one of --ts or --nts")
	}
	thread := phpenv.ThreadSafety("")
	if *nts {
		thread = phpenv.NonThreadSafe
	}
	if *ts {
		thread = phpenv.ThreadSafe
	}
	if thread == "" {
		thread = defaultThreadSafety(a.Config())
	}
	return finishUse(manager, phpenv.UseOptions{
		Version:      fs.Arg(0),
		Arch:         normalizedArch(*arch, a.Config()),
		ThreadSafety: thread,
		Description:  *desc,
	})
}

func finishUse(manager *phpenv.Manager, opts phpenv.UseOptions) error {
	if err := manager.Use(opts); err != nil {
		return err
	}
	resolved, err := manager.Which("")
	if err != nil {
		return err
	}
	printSuccess("Now using %s", resolved.PHPPath)
	if resolved.Source != "" {
		styleInfo.Printf("Source: %s\n", resolved.Source)
	}
	if runtime.GOOS != "windows" {
		script := filepath.Join(manager.Config().Root, "profile", "phpenv.sh")
		styleInfo.Printf("Source %s to update your shell session.\n", script)
	}
	return nil
}

func whichCmd(a *App, args []string) error {
	manager := a.Manager()
	dir := ""
	if len(args) > 0 {
		dir = args[0]
	}
	resolved, err := manager.Which(dir)
	if err != nil {
		printWarn("%v", err)
		return nil
	}
	fmt.Println(resolved.PHPPath)
	var details []string
	if resolved.Source != "" {
		details = append(details, resolved.Source)
	}
	if resolved.Selection.Description != "" {
		details = append(details, resolved.Selection.Description)
	}
	if resolved.Selection.Version != "" {
		details = append(details, resolved.Selection.Version)
	}
	if len(details) > 0 {
		styleInfo.Printf("(%s)\n", strings.Join(details, ", "))
	}
	return nil
}

func execCmd(a *App, args []string) error {
	if len(args) == 0 {
		return errors.New("exec requires the target command")
	}
	if strings.ToLower(args[0]) != "php" {
		return fmt.Errorf("unsupported exec target %q", args[0])
	}
	resolved, err := a.Manager().Resolve(phpenv.ResolveOptions{})
	if err != nil {
		return err
	}
	cmd := exec.Command(resolved.PHPPath, args[1:]...)
	cmd.Env = mergeEnvLists(os.Environ(), resolved.Env, resolved.PathAdditions)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func localCmd(a *App, args []string) error {
	manager := a.Manager()
	fs := newFlagSet("local")
	var (
		dir   = fs.String("dir", "", "Directory to write the override (default: current directory)")
		arch  = fs.String("arch", "", "Architecture (x64 or x86)")
		nts   = fs.Bool("nts", false, "Prefer non-thread-safe build")
		ts    = fs.Bool("ts", false, "Prefer thread-safe build")
		path  = fs.String("path", "", "Custom php executable for this directory")
		desc  = fs.String("desc", "", "Attach a friendly description")
		clear = fs.Bool("clear", false, "Remove the override file")
	)
	envFlag := keyValueFlag{}
	pathAdd := stringSliceFlag{}
	fs.Var(&envFlag, "env", "Environment variable KEY=VALUE (repeatable)")
	fs.Var(&pathAdd, "path-add", "Prepend to PATH (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *clear {
		return manager.SetLocal(phpenv.LocalOptions{Dir: *dir, Clear: true})
	}
	selection := config.Selection{}
	envMap := map[string]string{}
	for k, v := range envFlag {
		envMap[k] = v
	}
	if *path != "" {
		if fs.NArg() > 0 {
			return errors.New("when using --path do not provide a version argument")
		}
		selection.CustomPHP = *path
		selection.Description = *desc
	} else {
		if fs.NArg() < 1 {
			return errors.New("local requires a version argument or --path")
		}
		if *nts && *ts {
			return errors.New("specify only one of --ts or --nts")
		}
		thread := phpenv.ThreadSafety("")
		if *nts {
			thread = phpenv.NonThreadSafe
		}
		if *ts {
			thread = phpenv.ThreadSafe
		}
		if thread == "" {
			thread = defaultThreadSafety(a.Config())
		}
		slug, err := resolveInstalledSlug(manager, fs.Arg(0), normalizedArch(*arch, a.Config()), thread)
		if err != nil {
			return err
		}
		selection.Version = slug
		selection.Description = *desc
	}
	return manager.SetLocal(phpenv.LocalOptions{
		Dir:           *dir,
		Selection:     selection,
		Env:           envMap,
		PathAdditions: append([]string(nil), pathAdd...),
	})
}

func mergeEnvLists(base []string, overrides map[string]string, prependPaths []string) []string {
	env := make([]string, len(base))
	copy(env, base)
	index := map[string]int{}
	for i, entry := range env {
		if eq := strings.IndexRune(entry, '='); eq != -1 {
			index[strings.ToUpper(entry[:eq])] = i
		}
	}
	for key, value := range overrides {
		if key == "" {
			continue
		}
		entry := key + "=" + value
		upper := strings.ToUpper(key)
		if i, ok := index[upper]; ok {
			env[i] = entry
		} else {
			env = append(env, entry)
			index[upper] = len(env) - 1
		}
	}
	if len(prependPaths) > 0 {
		existing := ""
		if i, ok := index["PATH"]; ok {
			existing = envValue(env[i])
		} else {
			existing = os.Getenv("PATH")
		}
		entry := "PATH=" + phpenv.MergePathSegments(prependPaths, existing)
		if i, ok := index["PATH"]; ok {
			env[i] = entry
		} else {
			env = append(env, entry)
		}
	}
	return env
}

func envValue(entry string) string {
	if eq := strings.IndexRune(entry, '='); eq != -1 {
		return entry[eq+1:]
	}
	return ""
}

func normalizedArch(requested string, cfg *config.Config) string {
	if strings.TrimSpace(requested) != "" {
		return strings.ToLower(requested)
	}
	if strings.TrimSpace(cfg.DefaultArch) != "" {
		return strings.ToLower(cfg.DefaultArch)
	}
	return detectRuntimeArch()
}

func defaultThreadSafety(cfg *config.Config) phpenv.ThreadSafety {
	if strings.EqualFold(cfg.DefaultThreadSafety, "ts") {
		return phpenv.ThreadSafe
	}
	return phpenv.NonThreadSafe
}

func detectRuntimeArch() string {
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return "x64"
	case "386":
		return "x86"
	default:
		return ""
	}
}

func formatTS(ts phpenv.ThreadSafety) string {
	switch ts {
	case phpenv.ThreadSafe:
		return "ts"
	default:
		return "nts"
	}
}

func resolveInstalledSlug(manager *phpenv.Manager, version, arch string, ts phpenv.ThreadSafety) (string, error) {
	installed, err := manager.ListLocal()
	if err != nil {
		return "", err
	}
	if ts == "" {
		ts = phpenv.NonThreadSafe
	}
	arch = strings.ToLower(strings.TrimSpace(arch))
	version = strings.TrimSpace(version)
	for _, v := range installed {
		if !strings.EqualFold(v.Version, version) {
			continue
		}
		if arch != "" && v.Arch != "" && !strings.EqualFold(v.Arch, arch) {
			continue
		}
		if ts != "" && v.ThreadSafety != "" && !strings.EqualFold(string(v.ThreadSafety), string(ts)) {
			continue
		}
		return v.Name, nil
	}
	return "", fmt.Errorf("version %s (%s-%s) not installed", version, formatTS(ts), arch)
}

func printUsage() {
	printTitle("phpenv - PHP version manager")
	styleInfo.Println("Usage: phpenv <command> [options]")
	fmt.Println()
	styleSection.Println("Commands")
	max := 0
	for _, c := range commandCatalog {
		if len(c.Usage) > max {
			max = len(c.Usage)
		}
	}
	for _, c := range commandCatalog {
		styleHelpKey.Printf("  %-*s  ", max, c.Usage)
		styleHelpDesc.Printf("%s\n", c.Summary)
	}
	fmt.Println()
	styleInfo.Println("Run `phpenv help <command>` for detailed flag information.")
}

func printCommandHelp(name string) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "--help" || key == "-h" || key == "" {
		printUsage()
		return
	}
	cmd, ok := commandLookup[key]
	if !ok {
		printWarn("Unknown command: %s", name)
		fmt.Println()
		printUsage()
		return
	}
	printTitle("phpenv %s", cmd.Name)
	styleInfo.Printf("Usage: phpenv %s\n", cmd.Usage)
	styleInfo.Printf("Description: %s\n", cmd.Summary)
	if len(cmd.Flags) > 0 {
		fmt.Println()
		styleSection.Println("Flags")
		max := 0
		for _, f := range cmd.Flags {
			if len(f.Name) > max {
				max = len(f.Name)
			}
		}
		for _, f := range cmd.Flags {
			styleHelpKey.Printf("  %-*s  ", max, f.Name)
			styleHelpDesc.Printf("%s\n", f.Description)
		}
	}
}

type keyValueFlag map[string]string

func (kv *keyValueFlag) String() string {
	if kv == nil || len(*kv) == 0 {
		return ""
	}
	parts := make([]string, 0, len(*kv))
	for k, v := range *kv {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, ",")
}

func (kv *keyValueFlag) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected KEY=VALUE, got %q", value)
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return errors.New("environment key cannot be empty")
	}
	if *kv == nil {
		*kv = map[string]string{}
	}
	(*kv)[key] = parts[1]
	return nil
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, string(os.PathListSeparator))
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(new(strings.Builder))
	fs.Usage = func() { printCommandHelp(name) }
	return fs
}
