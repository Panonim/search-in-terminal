package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Panonim/search-in-terminal/internal/config"
)

func Run(cfg config.Config, version, query string) error {
	m, err := New(cfg, version, query)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}
