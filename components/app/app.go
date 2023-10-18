package app

import (
	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/app/components/profiling"
	"github.com/iotaledger/hive.go/app/components/shutdown"
	"github.com/iotaledger/inx-app/components/inx"
	"github.com/iotaledger/inx-coordinator/components/coordinator"
	"github.com/iotaledger/inx-coordinator/components/migrator"
)

var (
	// Name of the app.
	Name = "inx-coordinator"

	// Version of the app.
	Version = "1.0.1"
)

func App() *app.App {
	return app.New(Name, Version,
		app.WithInitComponent(InitComponent),
		app.WithComponents(
			inx.Component,
			coordinator.Component,
			shutdown.Component,
			migrator.Component,
			profiling.Component,
			//nolint:gocritic // false positive
			//prometheus.Plugin,
		),
	)
}

var (
	InitComponent *app.InitComponent
)

func init() {
	InitComponent = &app.InitComponent{
		Component: &app.Component{
			Name: "App",
		},
		NonHiddenFlags: []string{
			"config",
			"help",
			"version",
			"migratorBootstrap",
			"migratorStartIndex",
			"cooBootstrap",
			"cooStartIndex",
		},
	}
}
