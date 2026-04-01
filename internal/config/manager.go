package config

import (
	"log/slog"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Manager handles config loading and hot-reloading.
type Manager struct {
	path    string
	mu      sync.RWMutex
	current *Config
	watcher *fsnotify.Watcher
	logger  *slog.Logger

	// Callbacks for when config changes
	onReload []func(*Config)
}

// NewManager creates a config manager that watches for changes.
func NewManager(path string, logger *slog.Logger) (*Manager, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}

	m := &Manager{
		path:    path,
		current: cfg,
		logger:  logger,
	}

	return m, nil
}

// Config returns the current config (thread-safe).
func (m *Manager) Config() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// OnReload registers a callback to be called when config is reloaded.
func (m *Manager) OnReload(fn func(*Config)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onReload = append(m.onReload, fn)
}

// Reload manually reloads the config file.
func (m *Manager) Reload() error {
	cfg, err := Load(m.path)
	if err != nil {
		m.logger.Error("config reload failed", "error", err)
		return err
	}

	m.mu.Lock()
	m.current = cfg
	callbacks := m.onReload
	m.mu.Unlock()

	m.logger.Info("config reloaded successfully")

	// Notify callbacks
	for _, fn := range callbacks {
		fn(cfg)
	}

	return nil
}

// Watch starts watching the config file for changes.
func (m *Manager) Watch() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	m.watcher = watcher

	go m.watchLoop()

	if err := watcher.Add(m.path); err != nil {
		watcher.Close()
		return err
	}

	m.logger.Info("watching config file for changes", "path", m.path)
	return nil
}

func (m *Manager) watchLoop() {
	for {
		select {
		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}
			// Reload on write or create (some editors delete and recreate)
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				m.logger.Info("config file changed, reloading", "path", event.Name)
				if err := m.Reload(); err != nil {
					m.logger.Error("reload failed, keeping old config", "error", err)
				}
			}
		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			m.logger.Error("config watcher error", "error", err)
		}
	}
}

// Close stops watching the config file.
func (m *Manager) Close() error {
	if m.watcher != nil {
		return m.watcher.Close()
	}
	return nil
}
