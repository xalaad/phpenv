package phpenv

func (m *Manager) applySystemEnvironment() error {
	resolved, err := m.Resolve(ResolveOptions{ForceGlobal: true})
	if err != nil {
		return err
	}
	return setPersistentEnvironment(m.cfg, resolved)
}
