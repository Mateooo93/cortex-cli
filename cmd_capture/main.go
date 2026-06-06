package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	cortexdaemon "github.com/Mateooo93/cortex-cli/internal/daemon"
	"github.com/Mateooo93/cortex-cli/internal/ui"
	"os"
)

func main() {
	cfg, _ := cortexconfig.Load()
	cortexdaemon.SetGlobalConfigLoader(func() *cortexconfig.Config { return cfg })
	client := cortexdaemon.NewSessionClient("")
	client.Connect(os.TempDir(), "", cfg.DefaultModel, false, true, true, false)
	vixCfg, _ := config.Load(false, os.TempDir(), "", "")
	ui.Version = "cortex dev"
	// testMode=false so the welcome screen shows the CORTEX logo
	model := ui.NewModel(vixCfg, cfg, client, false, "", true, true)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 130, Height: 40})
	fmt.Print(updated.View().Content)
}
