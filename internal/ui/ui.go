package ui

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"phpenv/internal/phpenv"
)

// ErrExitRequested is returned when the user chooses to leave the interactive UI.
var ErrExitRequested = fmt.Errorf("ui exit requested")

type viewState int

const (
	viewMenu viewState = iota
	viewLocal
	viewRemote
	viewInstall
	viewUse
	viewRemove
	viewActive
	viewConfig
	viewMessage
	viewProgress
)

type menuItem struct {
	title string
	desc  string
	view  viewState
}

func (m menuItem) Title() string       { return m.title }
func (m menuItem) Description() string { return m.desc }
func (m menuItem) FilterValue() string { return m.title }

type localItem struct {
	version phpenv.LocalVersion
	active  bool
}

func (l localItem) Title() string {
	prefix := "  "
	if l.active {
		prefix = "* "
	}
	return fmt.Sprintf("%s%-10s %-3s %-4s %s", prefix, l.version.Version, strings.ToUpper(formatTS(l.version.ThreadSafety)), strings.ToUpper(l.version.Arch), l.version.Name)
}

func (l localItem) Description() string {
	return l.version.PHPPath
}

func (l localItem) FilterValue() string {
	return l.version.Name
}

type remoteItem struct {
	build phpenv.WinBuild
}

func (r remoteItem) Title() string {
	ts := "ts"
	if r.build.NTS {
		ts = "nts"
	}
	return fmt.Sprintf("%-10s %-3s %-4s %s", r.build.Version, ts, r.build.Arch, r.build.URL)
}

func (r remoteItem) Description() string {
	return "Download from windows.php.net"
}

func (r remoteItem) FilterValue() string {
	return r.build.Version + " " + r.build.Arch
}

type installProgressMsg struct {
	progress phpenv.DownloadProgress
}

type installResultMsg struct {
	err error
}

type remoteBuildsMsg struct {
	builds []phpenv.WinBuild
	err    error
}

type statusMsg string

type model struct {
	manager *phpenv.Manager

	state   viewState
	history []viewState

	menu         list.Model
	localList    list.Model
	useList      list.Model
	removeList   list.Model
	remoteList   list.Model
	remoteLoad   bool
	remoteError  error
	remoteFilter phpenv.RemoteFilter

	command     textinput.Model
	commandMode bool

	progressBar   progress.Model
	progressValue float64
	progressLabel string
	progressChan  <-chan phpenv.DownloadProgress
	resultChan    <-chan error

	spinner spinner.Model

	statusText string
	errText    string

	width  int
	height int
}

// Run launches the interactive application.
func Run(manager *phpenv.Manager) error {
	p := tea.NewProgram(newModel(manager), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	return ErrExitRequested
}

func newModel(manager *phpenv.Manager) model {
	menuItems := []list.Item{
		menuItem{"Installed Versions", "View installed PHP builds", viewLocal},
		menuItem{"Remote Builds", "Browse Windows builds", viewRemote},
		menuItem{"Install Version", "Download and install PHP", viewInstall},
		menuItem{"Use Version", "Activate an installed version", viewUse},
		menuItem{"Remove Version", "Delete an installed version", viewRemove},
		menuItem{"Active PHP", "Show current PHP details", viewActive},
		menuItem{"Configuration", "Show global configuration", viewConfig},
		menuItem{"Quit", "Exit phpenv", viewMessage},
	}
	menu := list.New(menuItems, list.NewDefaultDelegate(), 0, 0)
	menu.Title = "phpenv"
	menu.SetShowStatusBar(false)
	menu.SetFilteringEnabled(false)
	menu.SetShowHelp(false)

	localItems := make([]list.Item, 0)
	localList := list.New(localItems, list.NewDefaultDelegate(), 0, 0)
	localList.Title = "Installed"
	localList.SetShowStatusBar(false)
	localList.SetFilteringEnabled(true)
	localList.SetShowFilter(true)

	useList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	useList.Title = "Select version to use"
	useList.SetFilteringEnabled(true)
	useList.SetShowFilter(true)

	removeList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	removeList.Title = "Select version to remove"
	removeList.SetFilteringEnabled(true)
	removeList.SetShowFilter(true)

	remoteList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	remoteList.Title = "Remote builds"
	remoteList.SetFilteringEnabled(true)
	remoteList.SetShowStatusBar(false)
	remoteList.SetShowFilter(true)

	cmd := textinput.New()
	cmd.Prompt = ":"
	cmd.Placeholder = "install 8.2.12 --nts"

	spin := spinner.New()
	spin.Spinner = spinner.Dot

	progressBar := progress.New(progress.WithDefaultGradient())

	m := model{
		manager:      manager,
		state:        viewMenu,
		menu:         menu,
		localList:    localList,
		useList:      useList,
		removeList:   removeList,
		remoteList:   remoteList,
		command:      cmd,
		spinner:      spin,
		progressBar:  progressBar,
		statusText:   "Ready",
		remoteFilter: deriveDefaultFilter(manager),
	}
	m.refreshLocal()
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		availableHeight := max(10, m.height-8)
		m.menu.SetSize(m.width-4, availableHeight)
		m.localList.SetSize(m.width-4, availableHeight)
		m.useList.SetSize(m.width-4, availableHeight)
		m.removeList.SetSize(m.width-4, availableHeight)
		m.remoteList.SetSize(m.width-4, availableHeight)
	case tea.KeyMsg:
		if m.commandMode {
			switch msg.Type {
			case tea.KeyEsc:
				m.commandMode = false
				m.command.SetValue("")
			case tea.KeyEnter:
				input := strings.TrimSpace(m.command.Value())
				m.commandMode = false
				m.command.SetValue("")
				if input != "" {
					cmds = append(cmds, m.handleCommand(input))
				}
			default:
				var cmd tea.Cmd
				m.command, cmd = m.command.Update(msg)
				cmds = append(cmds, cmd)
			}
			break
		}

		switch msg.String() {
		case "ctrl+c", "q":
			if m.state == viewMenu {
				return m, tea.Quit
			}
			m.popView()
		case "esc", "b":
			if m.state == viewMenu {
				return m, tea.Quit
			}
			m.popView()
		case ":":
			m.commandMode = true
			m.command.SetValue("")
			m.command.Focus()
		default:
			switch m.state {
			case viewMenu:
				var cmd tea.Cmd
				m.menu, cmd = m.menu.Update(msg)
				cmds = append(cmds, cmd)
				if msg.Type == tea.KeyEnter {
					item, ok := m.menu.SelectedItem().(menuItem)
					if ok {
						if item.title == "Quit" {
							return m, tea.Quit
						}
						m.pushView(item.view)
						cmds = append(cmds, m.enterView(item.view))
					}
				}
			case viewLocal:
				var cmd tea.Cmd
				m.localList, cmd = m.localList.Update(msg)
				cmds = append(cmds, cmd)
			case viewUse:
				var cmd tea.Cmd
				m.useList, cmd = m.useList.Update(msg)
				cmds = append(cmds, cmd)
				if msg.Type == tea.KeyEnter {
					if item, ok := m.useList.SelectedItem().(localItem); ok {
						err := m.manager.Use(phpenv.UseOptions{
							Version:      item.version.Version,
							Arch:         item.version.Arch,
							ThreadSafety: item.version.ThreadSafety,
						})
						if err != nil {
							m.setError(err)
						} else {
							m.statusText = fmt.Sprintf("Activated %s", item.version.Name)
							m.refreshLocal()
						}
					}
				}
			case viewRemove:
				var cmd tea.Cmd
				m.removeList, cmd = m.removeList.Update(msg)
				cmds = append(cmds, cmd)
				if msg.Type == tea.KeyEnter {
					if item, ok := m.removeList.SelectedItem().(localItem); ok {
						err := m.manager.Remove(phpenv.RemoveOptions{
							Version:      item.version.Version,
							Arch:         item.version.Arch,
							ThreadSafety: item.version.ThreadSafety,
						})
						if err != nil {
							m.setError(err)
						} else {
							m.statusText = fmt.Sprintf("Removed %s", item.version.Name)
							m.refreshLocal()
						}
					}
				}
			case viewRemote:
				var cmd tea.Cmd
				m.remoteList, cmd = m.remoteList.Update(msg)
				cmds = append(cmds, cmd)
				if msg.Type == tea.KeyEnter {
					if item, ok := m.remoteList.SelectedItem().(remoteItem); ok {
						ts := phpenv.ThreadSafe
						if item.build.NTS {
							ts = phpenv.NonThreadSafe
						}
						cmds = append(cmds, m.startInstall(phpenv.InstallOptions{
							Version:      item.build.Version,
							Arch:         item.build.Arch,
							ThreadSafety: ts,
						}))
					}
				}
				if msg.String() == "r" {
					m.remoteLoad = true
					m.remoteError = nil
					m.remoteList.SetItems([]list.Item{})
					cmds = append(cmds, fetchRemoteCmd(m.manager, m.remoteFilter))
				}
			case viewInstall:
				if msg.Type == tea.KeyEnter {
					cmds = append(cmds, m.promptInstall())
				}
			}
		}
	case remoteBuildsMsg:
		m.remoteLoad = false
		m.remoteError = msg.err
		if msg.err == nil {
			items := make([]list.Item, 0, len(msg.builds))
			for _, b := range msg.builds {
				items = append(items, remoteItem{build: b})
			}
			m.remoteList.SetItems(items)
			m.statusText = fmt.Sprintf("Remote builds: %d results (%s %s)", len(items), strings.ToUpper(m.remoteFilter.Arch), strings.ToUpper(formatTS(m.remoteFilter.ThreadSafety)))
		}
	case installProgressMsg:
		if msg.progress.Total > 0 {
			percent := float64(msg.progress.Completed) / float64(msg.progress.Total)
			if percent > 1 {
				percent = 1
			}
			m.progressValue = percent
		}
		if m.progressChan != nil {
			cmds = append(cmds, waitForProgress(m.progressChan))
		}
	case installResultMsg:
		m.progressChan = nil
		m.resultChan = nil
		if msg.err != nil {
			m.setError(msg.err)
		} else {
			m.statusText = "Installation complete"
			m.refreshLocal()
		}
		m.popView()
	case statusMsg:
		m.statusText = string(msg)
	}

	spinModel, spinCmd := m.spinner.Update(msg)
	m.spinner = spinModel
	cmds = append(cmds, spinCmd)
	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	padding := lipgloss.NewStyle().Padding(1, 2)
	header := headerStyle.Render("phpenv · arrow keys to navigate · Enter select · Esc back · : command · q quit")

	var body string
	switch m.state {
	case viewMenu:
		body = m.menu.View()
	case viewLocal:
		if len(m.localList.Items()) == 0 {
			body = "No versions installed yet."
		} else {
			body = m.localList.View()
		}
	case viewRemote:
		if m.remoteLoad {
			body = spinnerStyle.Render(m.spinner.View() + " Loading remote builds…")
		} else if m.remoteError != nil {
			body = errorStyle.Render(m.remoteError.Error())
		} else {
			body = m.remoteList.View()
		}
	case viewUse:
		if len(m.useList.Items()) == 0 {
			body = "No versions installed yet."
		} else {
			body = m.useList.View()
		}
	case viewRemove:
		if len(m.removeList.Items()) == 0 {
			body = "Nothing to remove."
		} else {
			body = m.removeList.View()
		}
	case viewActive:
		body = m.renderActive()
	case viewConfig:
		body = m.renderConfig()
	case viewInstall:
		body = instructionStyle.Render("Press Enter to install latest defaults (use commands for custom options).")
	case viewMessage:
		if m.errText != "" {
			body = errorStyle.Render(m.errText)
		} else {
			body = successStyle.Render(m.statusText)
		}
	case viewProgress:
		bar := m.progressBar.ViewAs(m.progressValue)
		body = fmt.Sprintf("%s\n%s", m.progressLabel, bar)
	default:
		body = ""
	}

	footer := statusStyle.Render(m.statusText)
	if m.errText != "" {
		footer = errorStyle.Render(m.errText)
	}

	if m.commandMode {
		body += "\n" + commandStyle.Render(m.command.View())
	}

	return padding.Render(header + "\n\n" + body + "\n\n" + footer)
}

func (m *model) enterView(state viewState) tea.Cmd {
	switch state {
	case viewLocal:
		m.refreshLocal()
	case viewRemote:
		m.remoteLoad = true
		m.remoteError = nil
		m.remoteList.SetItems([]list.Item{})
		m.remoteFilter = deriveDefaultFilter(m.manager)
		m.statusText = fmt.Sprintf("Remote filter: %s %s", strings.ToUpper(m.remoteFilter.Arch), strings.ToUpper(formatTS(m.remoteFilter.ThreadSafety)))
		return fetchRemoteCmd(m.manager, m.remoteFilter)
	case viewInstall:
		m.progressValue = 0
	case viewActive:
		m.statusText = m.renderActive()
	case viewUse:
		m.refreshLocal()
		m.useList.SetItems(m.localList.Items())
	case viewRemove:
		m.refreshLocal()
		m.removeList.SetItems(m.localList.Items())
	case viewProgress:
		m.progressValue = 0
	}
	return nil
}

func (m *model) pushView(state viewState) {
	m.history = append(m.history, m.state)
	m.errText = ""
	m.state = state
}

func (m *model) popView() {
	if len(m.history) == 0 {
		m.state = viewMenu
		return
	}
	n := len(m.history) - 1
	m.state = m.history[n]
	m.history = m.history[:n]
	m.errText = ""
}

func (m *model) refreshLocal() {
	versions, err := m.manager.ListLocal()
	if err != nil {
		m.errText = err.Error()
		return
	}
	items := make([]list.Item, 0, len(versions))
	for _, v := range versions {
		items = append(items, localItem{version: v, active: v.Active})
	}
	m.localList.SetItems(items)
}

func (m *model) renderActive() string {
	resolved, err := m.manager.Which("")
	if err != nil {
		return "No active PHP set."
	}
	var details []string
	if resolved.Selection.Version != "" {
		details = append(details, resolved.Selection.Version)
	}
	if resolved.Selection.CustomPHP != "" {
		details = append(details, resolved.Selection.CustomPHP)
	}
	if resolved.Selection.Description != "" {
		details = append(details, resolved.Selection.Description)
	}
	if resolved.Source != "" {
		details = append(details, "source: "+resolved.Source)
	}
	lines := []string{
		"PHP Path: " + resolved.PHPPath,
		"Environment variables:",
	}
	for k, v := range resolved.Env {
		lines = append(lines, fmt.Sprintf("  %s=%s", k, v))
	}
	if len(resolved.PathAdditions) > 0 {
		lines = append(lines, "Prepended PATH entries:")
		for _, p := range resolved.PathAdditions {
			lines = append(lines, "  "+p)
		}
	}
	if len(details) > 0 {
		lines = append(lines, "Notes: "+strings.Join(details, " · "))
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderConfig() string {
	cfg := m.manager.Config()
	lines := []string{
		"Config:    " + cfg.FilePath(),
		"Root:      " + cfg.Root,
		"Versions:  " + cfg.VersionsDir,
		"Cache:     " + cfg.CacheDir,
		"Shims:     " + cfg.ShimsDir,
		"Default:   arch " + cfg.DefaultArch + " · thread " + cfg.DefaultThreadSafety,
	}
	if len(cfg.PathAdditions) > 0 {
		lines = append(lines, "PATH additions:")
		for _, p := range cfg.PathAdditions {
			lines = append(lines, "  "+p)
		}
	}
	if len(cfg.Env) > 0 {
		lines = append(lines, "Env overrides:")
		for k, v := range cfg.Env {
			lines = append(lines, fmt.Sprintf("  %s=%s", k, v))
		}
	}
	return strings.Join(lines, "\n")
}

func (m *model) promptInstall() tea.Cmd {
	m.commandMode = true
	m.command.SetValue("install ")
	m.command.CursorEnd()
	m.command.Focus()
	return nil
}

func (m *model) handleCommand(input string) tea.Cmd {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "quit", "exit", "q":
		return tea.Quit
	case "menu":
		m.state = viewMenu
	case "list":
		m.state = viewLocal
		m.refreshLocal()
	case "remote":
		m.pushView(viewRemote)
		return m.enterView(viewRemote)
	case "install":
		if len(fields) < 2 {
			m.setError(fmt.Errorf("install requires a version"))
			return nil
		}
		opts, err := m.parseInstallArgs(fields[1:])
		if err != nil {
			m.setError(err)
			return nil
		}
		return m.startInstall(opts)
	case "use":
		opts, err := m.parseUseArgs(fields[1:])
		if err != nil {
			m.setError(err)
			return nil
		}
		if err := m.manager.Use(opts); err != nil {
			m.setError(err)
		} else {
			m.statusText = "Activated PHP selection"
			m.refreshLocal()
		}
	case "remove":
		opts, err := m.parseUseArgs(fields[1:])
		if err != nil {
			m.setError(err)
			return nil
		}
		if opts.CustomPath != "" {
			m.setError(fmt.Errorf("remove only supports installed versions"))
			return nil
		}
		if err := m.manager.Remove(phpenv.RemoveOptions{
			Version:      opts.Version,
			Arch:         opts.Arch,
			ThreadSafety: opts.ThreadSafety,
		}); err != nil {
			m.setError(err)
		} else {
			m.statusText = "Removed version"
			m.refreshLocal()
		}
	default:
		m.statusText = "Unknown command: " + fields[0]
	}
	return nil
}

func (m *model) parseInstallArgs(args []string) (phpenv.InstallOptions, error) {
	cfg := m.manager.Config()
	opts := phpenv.InstallOptions{
		Version:      args[0],
		Arch:         cfg.DefaultArch,
		ThreadSafety: phpenv.NonThreadSafe,
	}
	for _, arg := range args[1:] {
		switch arg {
		case "--force":
			opts.ForceDownload = true
		case "--nts":
			opts.ThreadSafety = phpenv.NonThreadSafe
		case "--ts":
			opts.ThreadSafety = phpenv.ThreadSafe
		default:
			if strings.HasPrefix(arg, "--arch") {
				parts := strings.SplitN(arg, "=", 2)
				if len(parts) == 2 {
					opts.Arch = parts[1]
				}
			} else {
				return opts, fmt.Errorf("unknown install flag: %s", arg)
			}
		}
	}
	return opts, nil
}

func (m *model) parseUseArgs(args []string) (phpenv.UseOptions, error) {
	cfg := m.manager.Config()
	opts := phpenv.UseOptions{
		Arch:         cfg.DefaultArch,
		ThreadSafety: phpenv.NonThreadSafe,
	}
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--path="):
			opts.CustomPath = strings.TrimPrefix(arg, "--path=")
		case arg == "--path":
			return opts, fmt.Errorf("--path requires a value")
		case arg == "--nts":
			opts.ThreadSafety = phpenv.NonThreadSafe
		case arg == "--ts":
			opts.ThreadSafety = phpenv.ThreadSafe
		case strings.HasPrefix(arg, "--arch="):
			opts.Arch = strings.TrimPrefix(arg, "--arch=")
		case strings.HasPrefix(arg, "--desc="):
			opts.Description = strings.TrimPrefix(arg, "--desc=")
		default:
			if opts.Version == "" && opts.CustomPath == "" {
				opts.Version = arg
			} else {
				return opts, fmt.Errorf("unexpected argument: %s", arg)
			}
		}
	}
	if opts.Version == "" && opts.CustomPath == "" {
		return opts, fmt.Errorf("version or --path required")
	}
	return opts, nil
}

func (m *model) startInstall(opts phpenv.InstallOptions) tea.Cmd {
	m.pushView(viewProgress)
	m.progressValue = 0
	m.progressLabel = fmt.Sprintf("Installing %s (%s)", opts.Version, formatTS(opts.ThreadSafety))
	progressChan := make(chan phpenv.DownloadProgress)
	resultChan := make(chan error, 1)
	m.progressChan = progressChan
	m.resultChan = resultChan
	go func() {
		opts.Progress = func(dp phpenv.DownloadProgress) {
			progressChan <- dp
		}
		err := m.manager.Install(opts)
		close(progressChan)
		resultChan <- err
		close(resultChan)
	}()
	return tea.Batch(waitForProgress(progressChan), waitForResult(resultChan))
}

func (m *model) setError(err error) {
	if err == nil {
		m.errText = ""
		return
	}
	m.errText = err.Error()
	m.statusText = "Error"
}

func waitForProgress(ch <-chan phpenv.DownloadProgress) tea.Cmd {
	return func() tea.Msg {
		dp, ok := <-ch
		if !ok {
			return nil
		}
		return installProgressMsg{progress: dp}
	}
}

func waitForResult(ch <-chan error) tea.Cmd {
	return func() tea.Msg {
		err, ok := <-ch
		if !ok {
			return installResultMsg{err: nil}
		}
		return installResultMsg{err: err}
	}
}

func fetchRemoteCmd(manager *phpenv.Manager, filter phpenv.RemoteFilter) tea.Cmd {
	return func() tea.Msg {
		builds, err := manager.ListRemote(filter)
		return remoteBuildsMsg{builds: builds, err: err}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func formatTS(ts phpenv.ThreadSafety) string {
	switch ts {
	case phpenv.ThreadSafe:
		return "ts"
	default:
		return "nts"
	}
}

func deriveDefaultFilter(manager *phpenv.Manager) phpenv.RemoteFilter {
	cfg := manager.Config()
	arch := strings.ToLower(cfg.DefaultArch)
	if arch == "" {
		arch = detectArch()
	}
	ts := phpenv.NonThreadSafe
	if strings.EqualFold(cfg.DefaultThreadSafety, "ts") {
		ts = phpenv.ThreadSafe
	}
	return phpenv.RemoteFilter{Arch: arch, ThreadSafety: ts}
}

func detectArch() string {
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return "x64"
	case "386":
		return "x86"
	default:
		return ""
	}
}

var (
	headerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	statusStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	successStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	instructionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
	commandStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("219"))
	spinnerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("105"))
)
