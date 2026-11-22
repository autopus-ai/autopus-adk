//go:build !unix && !windows

package procgroup

import "time"

type platformState struct{}

func (g *Group) prepare() {}

func (g *Group) start() error { return g.cmd.Start() }

func (g *Group) close() error { return nil }

func (g *Group) terminate() {}

// kill falls back to the direct child where there are no process groups.
func (g *Group) kill(time.Duration) error { return g.cmd.Process.Kill() }
