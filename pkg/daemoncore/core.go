package daemoncore

import (
	"pv_hp_ctrl/config"
	"pv_hp_ctrl/pkg/myuplink"
	"pv_hp_ctrl/pkg/pv"
	"pv_hp_ctrl/pkg/state"
	"time"
)

type clientConfig struct {
	clientID     string
	clientSecret string
	redirectURI  string
}

// ClientProvider keeps one reusable myUplink client per daemon package.
//
// In Go, this is a typical alternative to a base class field: the shared
// infrastructure lives in a small helper type that both daemons compose.
type ClientProvider struct {
	client       *myuplink.Client
	clientConfig clientConfig
}

// Dependencies bundles the common inputs that every daemon needs for one cycle.
//
// Grouping them in one struct keeps the control flow explicit without forcing
// inheritance-like structures.
type Dependencies struct {
	Config *config.Config
	Client *myuplink.Client
}

// TimingState stores the two hysteresis timers used by the daemons.
//
// Both daemons follow the same pattern: one timestamp for "conditions met since"
// and one for "conditions not met since".
type TimingState struct {
	ConditionsMetSince    time.Time
	ConditionsNotMetSince time.Time
}

// RunTasks executes each task in the given order.
func RunTasks(tasks ...func()) {
	for _, task := range tasks {
		if task == nil {
			continue
		}

		task()
	}
}

// RunEveryMinute executes the callback immediately and then every minute.
func RunEveryMinute(run func()) {
	run()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		run()
	}
}

// Run starts the central daemon loop for the given tasks.
func Run(tasks ...func()) {
	RunTasksEveryMinute(tasks...)
}

// RunTasksEveryMinute executes the given tasks immediately and then every minute.
func RunTasksEveryMinute(tasks ...func()) {
	RunTasks(tasks...)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		RunTasks(tasks...)
	}
}

// LoadDependencies centralizes the boilerplate that both control daemons share:
// config loading and myUplink client lookup.
func LoadDependencies(configPath string, provider *ClientProvider) (*Dependencies, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	client := provider.Get(cfg)

	return &Dependencies{
		Config: cfg,
		Client: client,
	}, nil
}

func LoadEnergyData(configPath string) (*pv.PVData, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	return pv.GetData(cfg.PV.PowerURL, cfg.PV.SocURL, cfg.PV.ConsumptionURL, cfg.PV.Username, cfg.PV.Password)
}

// Get returns the cached myUplink client or recreates it if credentials changed.
func (p *ClientProvider) Get(cfg *config.Config) *myuplink.Client {
	nextConfig := clientConfig{
		clientID:     cfg.MyUplink.ClientID,
		clientSecret: cfg.MyUplink.ClientSecret,
		redirectURI:  cfg.MyUplink.RedirectURI,
	}

	if p.client == nil || p.clientConfig != nextConfig {
		p.client = myuplink.NewClient(cfg.MyUplink.ClientID, cfg.MyUplink.ClientSecret, cfg.MyUplink.RedirectURI)
		p.clientConfig = nextConfig
	}

	return p.client
}

// HysteresisStatus converts timer state into the UI/API representation.
func HysteresisStatus(total time.Duration, startedAt, now time.Time) state.HysteresisStatus {
	if total <= 0 || startedAt.IsZero() {
		return state.HysteresisStatus{}
	}

	remaining := total - now.Sub(startedAt)
	if remaining < 0 {
		remaining = 0
	}

	return state.HysteresisStatus{
		Active:           true,
		RemainingSeconds: int(remaining.Round(time.Second) / time.Second),
	}
}
